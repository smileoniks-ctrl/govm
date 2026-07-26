package model

import (
	"errors"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// handleKey processes a key press in the main TUI surface.
// The dependency update confirmation modal is handled separately
// in handleUpdateConfirmKey and short-circuits this path.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		return m.handleTabKey()
	case "shift+tab":
		return m.handleShiftTabKey()
	}
	if m.CurrentTab == SettingsTab {
		return m.handleSettingsKey(msg)
	}
	if m.CurrentTab == DepsTab && m.Deps.operationInProgress() {
		switch msg.String() {
		case "u", "r", "b":
			return m, nil
		}
	}
	if m.projection.operationPhase() == catalogOperationPhaseMutating {
		switch msg.String() {
		case "i", "u", "d", "p":
			return m, nil
		}
	}
	switch msg.String() {
	case "i":
		return m.handleInstallKey()
	case "u":
		return m.handleUseKey()
	case "r":
		return m.handleRefreshKey()
	case "b":
		return m.handleBackupsKey()
	case "d":
		return m.handleDeleteKey()
	case "p":
		return m.handlePruneKey()
	case "y", "Y":
		if m.PruneConfirming {
			return m.handlePruneConfirmYes()
		}
		return m.handleDeleteConfirmYes()
	case "n", "N":
		if m.PruneConfirming {
			return m.handlePruneConfirmNo()
		}
		return m.handleDeleteConfirmNo()
	}
	return m.handleActiveComponentKey(msg)
}

func (m *Model) handleActiveComponentKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "k", "j":
	default:
		return m, nil
	}

	switch m.CurrentTab {
	case AvailableTab:
		return m, m.projection.updateAvailable(msg)
	case InstalledTab:
		return m, m.projection.updateInstalled(msg)
	case DepsTab:
		var cmd tea.Cmd
		m.Deps.Table, cmd = m.Deps.Table.Update(msg)
		return m, cmd
	}
	return m, nil
}

// switchTab moves focus to the target tab and runs every side-effect
// that must fire on arrival regardless of direction: it tears down the
// previous tab's context (tab-scoped status + pending delete
// confirmation), lazy-loads the Deps tab on first visit, and asks the
// renderer to clear the screen when entering the Settings tab. Tab
// (forward) and Shift+Tab (reverse) both route through here so both
// directions share the same invariants.
func (m *Model) switchTab(target int) (tea.Model, tea.Cmd) {
	m.clearTabContext()
	m.CurrentTab = target
	// Lazy-load deps on first visit.
	if m.CurrentTab == DepsTab && !m.Deps.Loaded {
		m.Deps.Phase = OpChecking
		return m, ListModuleDependenciesCmd(m.Deps.ModuleDir)
	}
	if m.CurrentTab == SettingsTab {
		return m, tea.ClearScreen
	}
	return m, nil
}

func (m *Model) handleTabKey() (tea.Model, tea.Cmd) {
	return m.switchTab((m.CurrentTab + 1) % tabCount)
}

// handleShiftTabKey is the reverse-direction mirror of handleTabKey:
// it moves focus to the previous tab in cycle order, wrapping from
// Available back to Settings.
func (m *Model) handleShiftTabKey() (tea.Model, tea.Cmd) {
	return m.switchTab((m.CurrentTab - 1 + tabCount) % tabCount)
}

func (m *Model) handleInstallKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab || m.projection.operationPhase() != catalogOperationPhaseIdle {
		return m, nil
	}
	selected := m.projection.selectedAvailableItem()
	if selected == nil {
		return m, nil
	}
	v, ok := m.projection.lookup(selected.Name)
	if !ok || v.Installed {
		return m, nil
	}
	operation := m.projection.startMutation(catalogMutationInstall, v.Version)
	if operation.id == 0 {
		return m, nil
	}
	m.Status.SetGlobal("", "")
	return m, m.installVersionProgressCmd(operation.id, v)
}

func (m *Model) handleUseKey() (tea.Model, tea.Cmd) {
	if (m.CurrentTab == AvailableTab || m.CurrentTab == InstalledTab) &&
		m.projection.operationPhase() != catalogOperationPhaseIdle {
		return m, nil
	}
	if m.CurrentTab == AvailableTab {
		selected := m.projection.selectedAvailableItem()
		if selected != nil {
			v, ok := m.projection.lookup(selected.Name)
			if ok && v.Installed {
				operation := m.projection.startMutation(catalogMutationActivation, v.Version)
				if operation.id == 0 {
					return m, nil
				}
				m.Status.SetGlobal(fmt.Sprintf("Switching to Go %s...", v.Version), "info")
				return m, m.activateVersionCmd(operation.id, v.Version)
			}
		}
		m.Status.SetTab("You need to install this version first. Press 'i' to install.", "error")
		return m, nil
	}
	if m.CurrentTab == InstalledTab {
		row := m.projection.selectedInstalledItem()
		if len(row) == 0 {
			return m, nil
		}
		v, ok := m.projection.lookup(row[0])
		if !ok || !v.Installed {
			return m, nil
		}
		if v.Active {
			m.Status.SetTab(fmt.Sprintf("Go %s is already active.", v.Version), "info")
			return m, nil
		}
		operation := m.projection.startMutation(catalogMutationActivation, v.Version)
		if operation.id == 0 {
			return m, nil
		}
		m.Status.SetGlobal(fmt.Sprintf("Switching to Go %s...", v.Version), "info")
		return m, m.activateVersionCmd(operation.id, v.Version)
	}
	if m.CurrentTab == DepsTab && m.Deps.Loaded {
		return m.startUpdateCycle()
	}
	return m, nil
}

// startUpdateCycle begins a fresh dependency update cycle: it creates a
// new Cycle, feeds StartEvent, and returns the tea.Cmd that runs the
// initial check-updates intent through the execution seam. The update
// confirmation dialog only opens after the fresh check completes.
func (m *Model) startUpdateCycle() (tea.Model, tea.Cmd) {
	m.Deps.Cycle = deps.NewUpdateCycle()
	next, intent, err := m.Deps.Cycle.Handle(deps.StartEvent{ModuleDir: m.Deps.ModuleDir})
	if err != nil {
		m.Status.SetTab("Could not start update.", "error")
		return m, nil
	}
	m.Deps.Cycle = next
	m.Status.Clear()
	return m, m.cycleExecuteCmd(intent)
}

func (m *Model) handleRefreshKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab == DepsTab {
		m.Deps.Phase = OpChecking
		// Progress text comes from DepsState.SpinnerText() while the
		// phase is in-flight, so we only need to clear any stale status
		// here. Using Status.Clear (rather than Status.SetGlobal) keeps
		// the scope tab-local, which lets the DependenciesMsg handler
		// tear it down cleanly when the check finishes.
		m.Status.Clear()
		return m, CheckModuleDependencyUpdatesCmd(m.Deps.ModuleDir)
	}
	if m.refreshInFlight() {
		return m, nil
	}
	outcome := m.projection.startLoad(catalogLoadPurposeRefresh)
	m.Status.SetGlobal("", "")
	return m, LoadVersionsCmd(m.runtime, outcome.loadRequest)
}

func (m Model) refreshInFlight() bool {
	phase := m.projection.operationPhase()
	return phase == catalogOperationPhaseLoading ||
		phase == catalogOperationPhaseReconciling ||
		m.projection.refilterPending
}

func (m *Model) handleBackupsKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != DepsTab {
		return m, nil
	}
	m.Deps.Phase = OpLoadingBackups
	// Progress text comes from DepsState.SpinnerText() while the
	// phase is in-flight, so we only need to clear any stale status
	// here. Using Status.Clear (rather than Status.SetGlobal) keeps
	// the scope tab-local, which lets the DependencyBackupsMsg
	// handler tear it down cleanly when the load finishes.
	m.Status.Clear()
	return m, ListDependencyBackupsCmd(m.Deps.ModuleDir)
}

func (m *Model) handleDeleteKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab && m.CurrentTab != InstalledTab {
		return m, nil
	}
	if m.PruneConfirming || m.PrunePreviewing || m.PruneRunning {
		return m, nil
	}
	if m.projection.operationPhase() != catalogOperationPhaseIdle {
		return m, nil
	}
	version := ""
	if m.CurrentTab == AvailableTab {
		selected := m.projection.selectedAvailableItem()
		if selected == nil {
			return m, nil
		}
		version = selected.Name
	} else {
		row := m.projection.selectedInstalledItem()
		if len(row) == 0 {
			return m, nil
		}
		version = row[0]
	}
	v, ok := m.projection.lookup(version)
	if !ok || !v.Installed {
		if m.CurrentTab == AvailableTab {
			m.Status.SetTab("This version is not installed.", "error")
		}
		return m, nil
	}
	if v.Active {
		m.Status.SetTab("Cannot delete active version. Switch to another version first.", "error")
		return m, nil
	}
	m.ConfirmingDelete = true
	m.DeleteVersion = v.Version
	m.Status.SetTab(fmt.Sprintf("Are you sure you want to delete Go %s? Press Y to confirm, N to cancel.", v.Version), "warning")
	return m, nil
}

func (m *Model) handlePruneKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != InstalledTab ||
		m.projection.operationPhase() != catalogOperationPhaseIdle ||
		m.ConfirmingDelete || m.PruneConfirming || m.PrunePreviewing || m.PruneRunning {
		return m, nil
	}
	if m.previewPrune == nil {
		m.Status.SetTab("Prune service is not configured.", "error")
		return m, nil
	}
	m.PrunePreviewing = true
	m.Status.SetTab("Preparing prune plan...", "info")
	return m, m.previewPruneCmd()
}

func (m *Model) handlePruneConfirmYes() (tea.Model, tea.Cmd) {
	if !m.PruneConfirming || m.PruneRunning {
		return m, nil
	}
	m.PruneConfirming = false
	m.PruneRunning = true
	m.Status.SetGlobal("Pruning inactive Go versions...", "info")
	return m, m.pruneCmd()
}

func (m *Model) handlePruneConfirmNo() (tea.Model, tea.Cmd) {
	if !m.PruneConfirming {
		return m, nil
	}
	m.PruneConfirming = false
	m.PrunePlan = prune.Result{}
	m.Status.SetTab("Prune operation canceled.", "info")
	return m, nil
}

func (m *Model) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "up", "k":
		m.Settings.MoveUp()
	case "down", "j":
		m.Settings.MoveDown()
	case "enter", "space":
		if m.Settings.Cursor == 2 {
			return m, m.Settings.OpenDepsBackupLimitInput()
		}
		if m.Settings.Cursor == 3 {
			return m, m.Settings.OpenDistributionSourceInput()
		}
		cmd = m.toggleSelectedSetting()
	case "left", "h":
		if m.Settings.Cursor == 2 {
			m.adjustDepsBackupLimit(-1)
		} else if m.Settings.Cursor == 3 {
			return m, m.Settings.OpenDistributionSourceInput()
		} else {
			cmd = m.toggleSelectedSetting()
		}
	case "right", "l":
		if m.Settings.Cursor == 2 {
			m.adjustDepsBackupLimit(1)
		} else if m.Settings.Cursor == 3 {
			return m, m.Settings.OpenDistributionSourceInput()
		} else {
			cmd = m.toggleSelectedSetting()
		}
	}
	return m, cmd
}

func (m *Model) handleDistributionSourceInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.Settings.CheckingDistributionSource {
		if msg.String() == "esc" {
			outcome := m.projection.failLoad(
				m.Settings.DistributionSourceRequestID,
				errors.New("distribution source check canceled"),
			)
			m.Settings.CloseDistributionSourceInput()
			return m.handleCatalogOutcome(outcome)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.Settings.CloseDistributionSourceInput()
		return m, nil
	case "r":
		m.Settings.DistributionSourceInput.SetValue(config.DefaultDistributionSource)
		m.Settings.DistributionSourceInputErr = ""
		return m.beginDistributionSourceCheck()
	case "enter":
		if err := m.Settings.DistributionSourceInput.Err; err != nil {
			m.Settings.DistributionSourceInputErr = err.Error()
			return m, nil
		}
		return m.beginDistributionSourceCheck()
	default:
		var cmd tea.Cmd
		m.Settings.DistributionSourceInput, cmd = m.Settings.DistributionSourceInput.Update(msg)
		m.Settings.DistributionSourceInputErr = ""
		return m, cmd
	}
}

func (m *Model) beginDistributionSourceCheck() (tea.Model, tea.Cmd) {
	source, err := config.ValidateDistributionSource(m.Settings.DistributionSourceInput.Value())
	if err != nil {
		m.Settings.DistributionSourceInputErr = err.Error()
		return m, nil
	}
	outcome := m.projection.startLoad(catalogLoadPurposeRefresh)
	if outcome.kind != catalogProjectionOutcomeLoadStarted {
		m.Settings.DistributionSourceInputErr = "cannot check distribution source while another operation is active"
		return m, nil
	}
	m.Settings.CheckingDistributionSource = true
	m.Settings.DistributionSourceRequestID = outcome.loadRequest.ID
	m.Status.SetGlobal("Checking distribution source...", "warning")
	return m, ValidateDistributionSourceCmd(m.runtime, outcome.loadRequest, source)
}

func (m *Model) handleDistributionSourceValidation(msg distributionSourceValidatedMsg) (tea.Model, tea.Cmd) {
	if !m.Settings.CheckingDistributionSource ||
		msg.RequestID != m.Settings.DistributionSourceRequestID {
		return m, nil
	}
	if msg.Err != nil {
		m.Settings.CheckingDistributionSource = false
		m.Settings.DistributionSourceInputErr = msg.Err.Error()
		outcome := m.projection.failLoad(msg.RequestID, msg.Err)
		m.handleCatalogOutcome(outcome)
		return m, nil
	}

	previous := m.Settings.Values
	normalized, err := config.ValidateDistributionSource(msg.Source)
	if err != nil {
		m.Settings.DistributionSourceInputErr = err.Error()
		return m, nil
	}
	next := previous
	next.DistributionSource = normalized
	if err := config.Save(m.Settings.Path, next); err != nil {
		m.Settings.DistributionSourceInputErr = fmt.Sprintf("Failed to save settings: %v", err)
		m.Settings.CheckingDistributionSource = false
		outcome := m.projection.failLoad(msg.RequestID, err)
		m.handleCatalogOutcome(outcome)
		return m, nil
	}
	if err := m.runtime.Loader.SetDistributionSource(normalized); err != nil {
		_ = config.Save(m.Settings.Path, previous)
		m.Settings.DistributionSourceInputErr = fmt.Sprintf("Failed to apply settings: %v", err)
		m.Settings.CheckingDistributionSource = false
		outcome := m.projection.failLoad(msg.RequestID, err)
		m.handleCatalogOutcome(outcome)
		return m, nil
	}

	m.Settings.Values = next
	m.Settings.CloseDistributionSourceInput()
	outcome := m.projection.acceptLoad(msg.RequestID, msg.Versions)
	if outcome.kind == catalogProjectionOutcomeRejected {
		_ = config.Save(m.Settings.Path, previous)
		_ = m.runtime.Loader.SetDistributionSource(previous.DistributionSource)
		m.Settings.Values = previous
		m.Settings.OpenDistributionSourceInput()
		m.Settings.DistributionSourceInputErr = fmt.Sprintf("Failed to apply catalog: %v", outcome.err)
		return m, nil
	}
	m.Status.SetTab("Settings saved.", "info")
	return m, outcome.cmd
}

func (m *Model) toggleSelectedSetting() tea.Cmd {
	m.Settings.Values = config.Normalize(m.Settings.Values)
	var cmd tea.Cmd
	switch m.Settings.Cursor {
	case 0:
		if m.Settings.Values.DepsDisplay == config.DepsDisplayDirect {
			m.Settings.Values.DepsDisplay = config.DepsDisplayAll
		} else {
			m.Settings.Values.DepsDisplay = config.DepsDisplayDirect
		}
		m.updateDependencyTable()
	case 1:
		if m.Settings.Values.Theme == config.ThemeCurrent {
			m.Settings.Values.Theme = config.ThemeLight
		} else {
			m.Settings.Values.Theme = config.ThemeCurrent
		}
		cmd = m.applyRuntimeTheme()
	}
	m.saveSettings()
	return cmd
}

func (m *Model) adjustDepsBackupLimit(delta int) {
	limit := m.Settings.Values.DepsBackupLimit + delta
	if limit < config.MinDepsBackupLimit {
		limit = config.MaxDepsBackupLimit
	} else if limit > config.MaxDepsBackupLimit {
		limit = config.MinDepsBackupLimit
	}
	m.Settings.Values.DepsBackupLimit = limit
	m.saveSettings()
}

func (m *Model) handleDepsBackupLimitInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.Settings.CloseDepsBackupLimitInput()
		return m, nil
	case "enter":
		if err := m.Settings.DepsBackupLimitInput.Err; err != nil {
			m.Settings.DepsBackupLimitInputErr = err.Error()
			return m, nil
		}

		limit, err := strconv.Atoi(m.Settings.DepsBackupLimitInput.Value())
		if err != nil {
			m.Settings.DepsBackupLimitInputErr = "Enter a whole number."
			return m, nil
		}
		if err := config.ValidateDepsBackupLimit(limit); err != nil {
			m.Settings.DepsBackupLimitInputErr = err.Error()
			return m, nil
		}

		values := m.Settings.Values
		values.DepsBackupLimit = limit
		if err := config.Save(m.Settings.Path, values); err != nil {
			m.Settings.DepsBackupLimitInputErr = fmt.Sprintf("Failed to save settings: %v", err)
			return m, nil
		}

		m.Settings.Values = values
		m.Settings.CloseDepsBackupLimitInput()
		m.Status.SetTab("Settings saved.", "info")
		return m, nil
	default:
		var cmd tea.Cmd
		m.Settings.DepsBackupLimitInput, cmd = m.Settings.DepsBackupLimitInput.Update(msg)
		m.Settings.DepsBackupLimitInputErr = ""
		return m, cmd
	}
}

// applyRuntimeTheme rebuilds m.theme from the user's current settings
// value and propagates the new theme to every component that caches
// style values by value (Spinner, installedTable, Deps.Table, List
// delegate). It also forwards the theme to the catalog and, when the
// catalog accepts it, re-applies the version projection so the list
// items pick up the new pre-rendered titles. The returned tea.Cmd
// propagates the asynchronous refilter (if any) through the settings
// key flow. Replaces the previous "mutate package-level globals and
// hope readers pick them up" model with explicit value propagation.
func (m *Model) applyRuntimeTheme() tea.Cmd {
	t := styles.NewTheme(config.ThemeName(m.Settings.Values.Theme))
	m.theme = t
	m.Settings.ApplyTheme()
	m.Spinner.Style = t.SpinnerStyle
	m.Progress.FullColor = t.Primary
	m.Progress.EmptyColor = t.Muted
	m.Progress.PercentageStyle = lipgloss.NewStyle().Foreground(t.Info)
	m.Deps.Table.SetStyles(tableStyles(t))
	return m.projection.setTheme(t).cmd
}

func (m *Model) saveSettings() {
	if err := config.Save(m.Settings.Path, m.Settings.Values); err != nil {
		m.Status.SetTab(fmt.Sprintf("Failed to save settings: %v", err), "error")
		return
	}
	m.Status.SetTab("Settings saved.", "info")
}

func (m *Model) handleDeleteConfirmYes() (tea.Model, tea.Cmd) {
	if !m.ConfirmingDelete {
		return m, nil
	}
	if m.projection.operationPhase() != catalogOperationPhaseIdle {
		return m, nil
	}
	version := m.DeleteVersion
	target, ok := m.projection.lookup(version)
	if !ok {
		// Missing lookup: surface an error and never dispatch a zero
		// GoVersion to the deleter.
		m.ConfirmingDelete = false
		m.DeleteVersion = ""
		m.Status.SetTab(fmt.Sprintf("Go %s is no longer available to delete.", version), "error")
		return m, nil
	}
	if !target.Installed {
		m.ConfirmingDelete = false
		m.DeleteVersion = ""
		m.Status.SetTab(fmt.Sprintf("Go %s is no longer installed.", version), "info")
		return m, nil
	}
	if target.Active {
		m.ConfirmingDelete = false
		m.DeleteVersion = ""
		m.Status.SetTab("Cannot delete active version. Switch to another version first.", "error")
		return m, nil
	}
	m.ConfirmingDelete = false
	m.DeleteVersion = ""
	operation := m.projection.startMutation(catalogMutationDeletion, target.Version)
	if operation.id == 0 {
		return m, nil
	}
	m.Status.SetGlobal(fmt.Sprintf("Deleting Go %s...", target.Version), "info")
	return m, m.deleteVersionCmd(operation.id, target.Version)
}

func (m *Model) handleDeleteConfirmNo() (tea.Model, tea.Cmd) {
	if !m.ConfirmingDelete {
		return m, nil
	}
	m.ConfirmingDelete = false
	m.DeleteVersion = ""
	m.Status.SetTab("Delete operation canceled.", "info")
	return m, nil
}
