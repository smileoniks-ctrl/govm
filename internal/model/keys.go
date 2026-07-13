package model

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// handleKey processes a key press in the main TUI surface.
// The dependency update confirmation modal is handled separately
// in handleUpdateConfirmKey and short-circuits this path.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		return m.handleTabKey()
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
	case "y", "Y":
		return m.handleDeleteConfirmYes()
	case "n", "N":
		return m.handleDeleteConfirmNo()
	}
	return m.handleActiveComponentKey(msg)
}

func (m Model) handleActiveComponentKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "k", "j":
	default:
		return m, nil
	}

	switch m.CurrentTab {
	case AvailableTab:
		var cmd tea.Cmd
		m.List, cmd = m.List.Update(msg)
		return m, cmd
	case InstalledTab:
		var cmd tea.Cmd
		m.InstalledTable, cmd = m.InstalledTable.Update(msg)
		return m, cmd
	case DepsTab:
		var cmd tea.Cmd
		m.Deps.Table, cmd = m.Deps.Table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleTabKey() (tea.Model, tea.Cmd) {
	m.clearTabContext()
	m.CurrentTab = (m.CurrentTab + 1) % tabCount
	// Lazy-load deps on first visit.
	if m.CurrentTab == DepsTab && !m.Deps.Loaded {
		m.Deps.Checking = true
		return m, utils.ListModuleDependencies(m.Deps.ModuleDir)
	}
	if m.CurrentTab == SettingsTab {
		return m, tea.ClearScreen
	}
	return m, nil
}

func (m Model) handleInstallKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab {
		return m, nil
	}
	selected := m.selectedItem()
	if selected == nil {
		return m, nil
	}
	for _, v := range m.Versions {
		if v.Version == selected.Name && !v.Installed {
			m.Loading = true
			m.InstallingVersion = v.Version
			m.setGlobalStatus("", "")
			return m, utils.DownloadAndInstall(v)
		}
	}
	return m, nil
}

func (m Model) handleUseKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab == AvailableTab {
		selected := m.selectedItem()
		if selected != nil {
			for _, v := range m.Versions {
				if v.Version == selected.Name && v.Installed {
					m.Loading = true
					m.setGlobalStatus(fmt.Sprintf("Switching to Go %s...", v.Version), "info")
					return m, utils.SwitchVersion(v)
				}
			}
		}
		m.setTabStatus("You need to install this version first. Press 'i' to install.", "error")
		return m, nil
	}
	if m.CurrentTab == DepsTab && m.Deps.Loaded && !m.Deps.Updating {
		updatable := utils.UpdatableDirectDependencies(m.Deps.Dependencies)
		if len(updatable) == 0 {
			m.setTabStatus("No direct dependency updates available.", "warning")
			return m, nil
		}
		m.Deps.Dialog.ConfirmingUpdate = true
		m.Deps.Dialog.UpdateChoiceYes = true
		m.clearStatus()
		return m, nil
	}
	return m, nil
}

func (m Model) handleRefreshKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab == DepsTab {
		m.Deps.Checking = true
		m.setGlobalStatus("Checking for dependency updates...", "info")
		return m, utils.CheckModuleDependencyUpdates(m.Deps.ModuleDir)
	}
	m.Loading = true
	m.setGlobalStatus("", "")
	return m, utils.FetchGoVersions
}

func (m Model) handleBackupsKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != DepsTab || m.Deps.LoadingBackups || m.Deps.RestoringBackup {
		return m, nil
	}
	m.Deps.LoadingBackups = true
	m.setGlobalStatus("Loading dependency backups...", "info")
	return m, utils.ListDependencyBackupsCmd(m.Deps.ModuleDir)
}

func (m Model) handleDeleteKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab != AvailableTab && m.CurrentTab != InstalledTab {
		return m, nil
	}
	selected := m.selectedItem()
	if selected == nil {
		return m, nil
	}
	for _, v := range m.Versions {
		if v.Version == selected.Name && v.Installed {
			if v.Active {
				m.setTabStatus("Cannot delete active version. Switch to another version first.", "error")
				return m, nil
			}
			m.ConfirmingDelete = true
			m.DeleteVersion = v.Version
			m.setTabStatus(fmt.Sprintf("Are you sure you want to delete Go %s? Press Y to confirm, N to cancel.", v.Version), "warning")
			return m, nil
		}
	}
	if m.CurrentTab == AvailableTab {
		m.setTabStatus("This version is not installed.", "error")
	}
	return m, nil
}

func (m Model) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.Settings.MoveUp()
	case "down", "j":
		m.Settings.MoveDown()
	case "enter", "space":
		if m.Settings.Cursor == 2 {
			return m, m.Settings.OpenDepsBackupLimitInput()
		}
		m.toggleSelectedSetting()
	case "left", "h":
		if m.Settings.Cursor == 2 {
			m.adjustDepsBackupLimit(-1)
		} else {
			m.toggleSelectedSetting()
		}
	case "right", "l":
		if m.Settings.Cursor == 2 {
			m.adjustDepsBackupLimit(1)
		} else {
			m.toggleSelectedSetting()
		}
	}
	return m, nil
}

func (m *Model) toggleSelectedSetting() {
	m.Settings.Values = config.Normalize(m.Settings.Values)
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
		m.applyRuntimeTheme()
	}
	m.saveSettings()
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

func (m Model) handleDepsBackupLimitInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.setTabStatus("Settings saved.", "info")
		return m, nil
	default:
		var cmd tea.Cmd
		m.Settings.DepsBackupLimitInput, cmd = m.Settings.DepsBackupLimitInput.Update(msg)
		m.Settings.DepsBackupLimitInputErr = ""
		return m, cmd
	}
}

func (m *Model) applyRuntimeTheme() {
	ApplyTheme(styles.ThemeName(m.Settings.Values.Theme))
	m.Settings.ApplyTheme()
	m.Spinner.Style = styles.SpinnerStyle
	m.InstalledTable.SetStyles(tableStyles())
	m.Deps.Table.SetStyles(tableStyles())
	delegate := listDefaultDelegate()
	m.List.SetDelegate(delegate)
}

func (m *Model) saveSettings() {
	if err := config.Save(m.Settings.Path, m.Settings.Values); err != nil {
		m.setTabStatus(fmt.Sprintf("Failed to save settings: %v", err), "error")
		return
	}
	m.setTabStatus("Settings saved.", "info")
}

func (m Model) handleDeleteConfirmYes() (tea.Model, tea.Cmd) {
	if !m.ConfirmingDelete {
		return m, nil
	}
	m.ConfirmingDelete = false
	m.Loading = true
	m.setGlobalStatus(fmt.Sprintf("Deleting Go %s...", m.DeleteVersion), "info")

	var target utils.GoVersion
	for _, v := range m.Versions {
		if v.Version == m.DeleteVersion {
			target = v
			break
		}
	}
	return m, utils.DeleteVersion(target)
}

func (m Model) handleDeleteConfirmNo() (tea.Model, tea.Cmd) {
	if !m.ConfirmingDelete {
		return m, nil
	}
	m.ConfirmingDelete = false
	m.DeleteVersion = ""
	m.setTabStatus("Delete operation canceled.", "info")
	return m, nil
}

func (m Model) selectedItem() *styles.Item {
	if m.List.SelectedItem() == nil {
		return nil
	}
	item, ok := m.List.SelectedItem().(styles.Item)
	if !ok {
		return nil
	}
	return &item
}
