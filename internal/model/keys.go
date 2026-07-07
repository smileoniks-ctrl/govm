package model

import (
	"fmt"

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
	switch msg.String() {
	case "i":
		return m.handleInstallKey()
	case "u":
		return m.handleUseKey()
	case "r":
		return m.handleRefreshKey()
	case "d":
		return m.handleDeleteKey()
	case "y", "Y":
		return m.handleDeleteConfirmYes()
	case "n", "N":
		return m.handleDeleteConfirmNo()
	}
	return m, nil
}

func (m Model) handleTabKey() (tea.Model, tea.Cmd) {
	m.CurrentTab = (m.CurrentTab + 1) % tabCount
	// Lazy-load deps on first visit.
	if m.CurrentTab == DepsTab && !m.Deps.Loaded {
		m.Deps.Checking = true
		return m, utils.ListModuleDependencies(m.Deps.ModuleDir)
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
			m.Message = ""
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
					m.Message = fmt.Sprintf("Switching to Go %s...", v.Version)
					return m, utils.SwitchVersion(v)
				}
			}
		}
		m.Message = "You need to install this version first. Press 'i' to install."
		m.MessageType = "error"
		return m, nil
	}
	if m.CurrentTab == DepsTab && m.Deps.Loaded && !m.Deps.Updating {
		updatable := utils.UpdatableDirectDependencies(m.Deps.Dependencies)
		if len(updatable) == 0 {
			m.Message = "No direct dependency updates available."
			m.MessageType = "warning"
			return m, nil
		}
		m.Deps.Dialog.ConfirmingUpdate = true
		m.Deps.Dialog.UpdateChoiceYes = true
		m.Message = ""
		m.MessageType = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleRefreshKey() (tea.Model, tea.Cmd) {
	if m.CurrentTab == DepsTab {
		m.Deps.Checking = true
		m.Message = "Checking for dependency updates..."
		m.MessageType = "info"
		return m, utils.CheckModuleDependencyUpdates(m.Deps.ModuleDir)
	}
	m.Loading = true
	m.Message = ""
	return m, utils.FetchGoVersions
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
				m.Message = "Cannot delete active version. Switch to another version first."
				m.MessageType = "error"
				return m, nil
			}
			m.ConfirmingDelete = true
			m.DeleteVersion = v.Version
			m.Message = fmt.Sprintf("Are you sure you want to delete Go %s? Press Y to confirm, N to cancel.", v.Version)
			m.MessageType = "warning"
			return m, nil
		}
	}
	if m.CurrentTab == AvailableTab {
		m.Message = "This version is not installed."
		m.MessageType = "error"
	}
	return m, nil
}

func (m Model) handleSettingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.Settings.MoveUp()
	case "down", "j":
		m.Settings.MoveDown()
	case "enter", "space", "left", "right", "h", "l":
		m.toggleSelectedSetting()
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

func (m *Model) applyRuntimeTheme() {
	ApplyTheme(styles.ThemeName(m.Settings.Values.Theme))
	m.Spinner.Style = styles.SpinnerStyle
	m.InstalledTable.SetStyles(tableStyles())
	m.Deps.Table.SetStyles(tableStyles())
	delegate := listDefaultDelegate()
	m.List.SetDelegate(delegate)
}

func (m *Model) saveSettings() {
	if err := config.Save(m.Settings.Path, m.Settings.Values); err != nil {
		m.Message = fmt.Sprintf("Failed to save settings: %v", err)
		m.MessageType = "error"
		return
	}
	m.Message = "Settings saved."
	m.MessageType = "info"
}

func (m Model) handleDeleteConfirmYes() (tea.Model, tea.Cmd) {
	if !m.ConfirmingDelete {
		return m, nil
	}
	m.ConfirmingDelete = false
	m.Loading = true
	m.Message = fmt.Sprintf("Deleting Go %s...", m.DeleteVersion)
	m.MessageType = "info"

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
	m.Message = "Delete operation canceled."
	m.MessageType = "info"
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
