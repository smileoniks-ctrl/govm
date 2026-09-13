package model

import (
	"errors"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// handleKey processes a key press in the main TUI surface: the Input
// context is the tab, the filter input or the inline delete
// confirmation. Dialogs, the prune confirmation, the Help overlay and
// the Settings inputs are dispatched before this path by update().
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	ctx := m.inputContext()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		// In a text-entry context q is ordinary input and must reach
		// the input instead of quitting.
		if !ctx.textEntry() {
			return m, tea.Quit
		}
	case "tab":
		return m.handleTabKey()
	case "shift+tab":
		return m.handleShiftTabKey()
	}
	// The filter input owns the keyboard while it has focus: every
	// remaining key is delivered to the list, which forwards it to
	// the input. Commands stay unreachable until the input closes.
	if ctx == inputFilter {
		return m, m.projection.updateAvailable(msg)
	}
	if m.CurrentTab == SettingsTab {
		return m.handleSettingsKey(msg)
	}
	if m.projection.operationPhase() == catalogOperationPhaseMutating {
		switch msg.String() {
		case "i", "u", "d", "p":
			return m, nil
		}
	}
	// The Deps tab owns every remaining key while it is current.
	if m.CurrentTab == DepsTab {
		return m.delegateDeps(msg)
	}
	switch msg.String() {
	case "i":
		return m.handleInstallKey()
	case "u":
		return m.handleUseKey()
	case "r":
		return m.handleRefreshKey()
	case "d":
		return m.handleDeleteKey()
	case "p":
		return m.handlePruneKey()
	case "f":
		return m.handleFilterKey(msg)
	case "esc":
		return m.handleFilterClearKey(msg)
	case "y", "Y":
		return m.handleDeleteConfirmYes()
	case "n", "N":
		return m.handleDeleteConfirmNo()
	}
	return m.handleActiveComponentKey(msg)
}

// handlePruneDialogKey owns the keyboard while the prune dialog awaits
// its answer. It mirrors handleDialogKey: the shared Yes/No keys move
// the highlight or commit a choice, and the per-kind confirm/cancel
// paths run the transition.
func (m *Model) handlePruneDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	choiceYes, action := yesNoKeyAction(msg.String(), m.Prune.ChoiceYes())
	m.Prune.SetChoiceYes(choiceYes)
	switch action {
	case DialogConfirm:
		return m.handlePruneConfirmYes()
	case DialogCancel:
		return m.handlePruneConfirmNo()
	}
	return m, nil
}

// handleHelpOverlayKey owns the keyboard while the Help overlay is
// open: ? and esc close it, ctrl+c still quits, and every other key
// is swallowed so neither the tab underneath nor an open dialog
// reacts while the overlay is showing.
func (m *Model) handleHelpOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc":
		m.HelpVisible = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
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
	}
	return m, nil
}

// filterInputActive reports whether the Available tab's filter input
// has focus. The input is a keyboard-owning mode: while active, every
// ordinary key (including q and ?) is filter text rather than a
// command. Switching tabs suspends, but does not cancel, the mode.
//
// The pointer receiver is deliberate: this runs on every key press,
// and a value receiver would copy the entire Model each time.
func (m *Model) filterInputActive() bool {
	return m.CurrentTab == AvailableTab && m.projection.availableSettingFilter()
}

// handleFilterKey opens the Available list's inline filter input by
// handing the key to the list widget (its Filter binding, rebound to "f"). The key is
// inert off the Available tab and whenever no new mode may open:
// while another Input context owns the keyboard (a pending inline
// confirmation) and below the minimum terminal size where the input
// would not render. An empty catalog needs no guard of its own: the
// widget disables its filter binding.
func (m *Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab || !m.canOpenMode() {
		return m, nil
	}
	return m, m.projection.updateAvailable(msg)
}

// handleFilterClearKey routes esc to the list while a committed filter
// narrows it, clearing the filter. esc is inert on the main surface in
// every other state, so the key is claimed only here.
func (m *Model) handleFilterClearKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.CurrentTab == AvailableTab && m.projection.availableFilterApplied() {
		return m, m.projection.updateAvailable(msg)
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
	if m.CurrentTab == DepsTab {
		cmd, status := m.Deps.enter()
		m.applyDepsStatus(status)
		return m, cmd
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
	return m, nil
}

func (m *Model) handleRefreshKey() (tea.Model, tea.Cmd) {
	if m.refreshInFlight() {
		return m, nil
	}
	outcome := m.projection.startLoad(catalogLoadPurposeRefresh)
	m.Status.SetGlobal("", "")
	return m, LoadVersionsCmd(m.loadCatalog, outcome.loadRequest)
}

func (m Model) refreshInFlight() bool {
	phase := m.projection.operationPhase()
	return phase == catalogOperationPhaseLoading ||
		phase == catalogOperationPhaseReconciling ||
		m.projection.refilterPending
}

func (m *Model) handleDeleteKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab && m.CurrentTab != InstalledTab {
		return m, nil
	}
	if m.Prune.Busy() {
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
		m.ConfirmingDelete {
		return m, nil
	}
	// The nil check stays ahead of the transition: moving to previewing
	// without emitting a command would strand the phase until the user
	// leaves the tab.
	if m.previewPrune == nil {
		m.Status.SetTab("Prune service is not configured.", "error")
		return m, nil
	}
	if !m.Prune.BeginPreview() {
		return m, nil
	}
	m.Status.SetTab("Preparing prune plan...", "info")
	return m, m.previewPruneCmd()
}

func (m *Model) handlePruneConfirmYes() (tea.Model, tea.Cmd) {
	if !m.Prune.Confirm() {
		return m, nil
	}
	m.Status.SetGlobal("Pruning inactive Go versions...", "info")
	return m, m.pruneCmd()
}

func (m *Model) handlePruneConfirmNo() (tea.Model, tea.Cmd) {
	if !m.Prune.Cancel() {
		return m, nil
	}
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
	return m, ChangeDistributionSourceCmd(m.distributionSource, outcome.loadRequest, source)
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
	next := previous
	next.DistributionSource = msg.Result.Source
	m.Settings.Values = next
	m.Settings.CloseDistributionSourceInput()
	outcome := m.projection.acceptLoad(msg.RequestID, msg.Result.Catalog.Versions)
	if outcome.kind == catalogProjectionOutcomeRejected {
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
		m.syncDepsSettings()
	case 1:
		if m.Settings.Values.Theme == config.ThemeCurrent {
			m.Settings.Values.Theme = config.ThemeLight
		} else {
			m.Settings.Values.Theme = config.ThemeCurrent
		}
		cmd = m.applyRuntimeTheme()
	case 4:
		if m.Settings.Values.UpgradeNotice == config.UpgradeNoticeOn {
			m.Settings.Values.UpgradeNotice = config.UpgradeNoticeOff
			m.upgradeNotice = ""
		} else {
			m.Settings.Values.UpgradeNotice = config.UpgradeNoticeOn
			cmd = m.startUpgradeCheck()
		}
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
	m.syncDepsSettings()
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
