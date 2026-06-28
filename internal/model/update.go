package model

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.Deps.Dialog.ConfirmingUpdate {
			return m.handleUpdateConfirmKey(msg)
		}
		if m.Deps.Dialog.ConfirmingChecks {
			return m.handleChecksConfirmKey(msg)
		}
		if m.Deps.Dialog.ConfirmingRollback {
			return m.handleRollbackConfirmKey(msg)
		}
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		m.Layout = styles.GetLayoutMode(msg.Width)

		frameH, frameV := styles.FrameOverhead(m.Layout)
		contentWidth := msg.Width - frameH
		if contentWidth < styles.MinTermWidth {
			contentWidth = styles.MinTermWidth
		}

		const fixedUIElements = 6
		contentHeight := msg.Height - frameV - fixedUIElements
		if contentHeight < styles.MinTermHeight {
			contentHeight = styles.MinTermHeight
		}

		m.Width = contentWidth
		m.Height = contentHeight
		m.List.SetSize(contentWidth, contentHeight)
		m.InstalledTable.SetWidth(contentWidth)
		m.InstalledTable.SetHeight(contentHeight)
		m.InstalledTable.SetColumns(installedTableColumns(contentWidth, m.Layout))
		m.Deps.Table.SetWidth(contentWidth)
		m.Deps.Table.SetHeight(contentHeight)
		m.Deps.Table.SetColumns(dependencyTableColumns(contentWidth, m.Layout))
		return m, nil

	case utils.ErrMsg:
		m.Err = nil
		m.Loading = false
		m.Message = msg.Error()
		m.MessageType = "error"
		return m, nil

	case utils.VersionsMsg:
		m.Err = nil
		m.Versions = msg
		items := make([]list.Item, len(m.Versions))
		for i, v := range m.Versions {
			items[i] = styles.Item{
				Name:            v.Version,
				DescriptionText: "go" + v.Version + " " + v.Filename,
				Installed:       v.Installed,
				Active:          v.Active,
			}
		}
		m.List.SetItems(items)
		m.Loading = false
		m.updateInstalledTable()
		return m, nil

	case utils.DependenciesMsg:
		m.Deps.Dependencies = msg
		m.Deps.Loaded = true
		m.Deps.Checking = false
		m.updateDependencyTable()
		m.Message = ""
		m.MessageType = ""
		return m, nil

	case utils.DependenciesUpdatedMsg:
		m.Deps.Updating = false
		m.Deps.Dependencies = msg.Dependencies
		m.Deps.Snapshot = msg.Snapshot
		m.Deps.LastCheckResult = nil
		m.updateDependencyTable()
		m.Message = fmt.Sprintf("Updated %d direct %s. Run checks?", msg.Updated, pluralize(msg.Updated, "dependency", "dependencies"))
		m.MessageType = "success"
		m.Deps.Dialog.ConfirmingChecks = true
		m.Deps.Dialog.CheckChoiceYes = true
		return m, nil

	case utils.DependencyCheckResultMsg:
		m.Deps.RunningChecks = false
		m.Deps.Dialog.ConfirmingChecks = false
		if msg.OK {
			m.Message = "Checks passed."
			m.MessageType = "success"
			m.Deps.LastCheckResult = &msg
			return m, nil
		}
		m.Deps.LastCheckResult = &msg
		m.Deps.Dialog.ConfirmingRollback = true
		m.Deps.Dialog.RollbackChoiceYes = true
		m.Message = fmt.Sprintf("Checks failed: %s", msg.Command)
		m.MessageType = "error"
		return m, nil

	case utils.DependenciesRolledBackMsg:
		m.Deps.RollingBack = false
		m.Deps.Dependencies = msg.Dependencies
		m.Deps.Snapshot = msg.Snapshot
		m.Deps.LastCheckResult = nil
		m.updateDependencyTable()
		m.Message = "Rolled back to pre-update state."
		m.MessageType = "success"
		return m, nil

	case utils.DependencyErrMsg:
		if m.Deps.Updating {
			m.Deps.Updating = false
		}
		if m.Deps.Checking {
			m.Deps.Checking = false
		}
		if m.Deps.RunningChecks {
			m.Deps.RunningChecks = false
		}
		if m.Deps.RollingBack {
			m.Deps.RollingBack = false
		}
		if msg.Err != nil {
			m.Message = msg.Err.Error()
		}
		m.MessageType = "error"
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case utils.DownloadCompleteMsg:
		m.Loading = false
		m.InstallingVersion = ""
		for i, v := range m.Versions {
			if v.Version == msg.Version {
				m.Versions[i].Installed = true
				m.Versions[i].Path = msg.Path
				break
			}
		}
		items := m.List.Items()
		for i, it := range items {
			if it.(styles.Item).Name == msg.Version {
				updatedItem := it.(styles.Item)
				updatedItem.Installed = true
				items[i] = updatedItem
			}
		}
		m.List.SetItems(items)
		m.updateInstalledTable()
		m.Message = fmt.Sprintf("Successfully installed Go %s", msg.Version)
		m.MessageType = "success"
		return m, nil

	case utils.SwitchCompletedMsg:
		m.Loading = false
		for i := range m.Versions {
			m.Versions[i].Active = (m.Versions[i].Version == msg.Version)
		}
		items := m.List.Items()
		for i, it := range items {
			updatedItem := it.(styles.Item)
			updatedItem.Active = (updatedItem.Name == msg.Version)
			items[i] = updatedItem
		}
		m.List.SetItems(items)
		m.updateInstalledTable()
		if msg.ShimInPath {
			m.Message = fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", msg.Version)
		} else {
			m.Message = fmt.Sprintf("Switched to Go %s!\n\n%s",
				msg.Version, utils.GetShimPathInstructions())
		}
		m.MessageType = "success"
		return m, nil

	case utils.DeleteCompleteMsg:
		m.Loading = false
		for i, v := range m.Versions {
			if v.Version == msg.Version {
				m.Versions[i].Installed = false
				m.Versions[i].Path = ""
				break
			}
		}
		items := m.List.Items()
		for i, it := range items {
			if it.(styles.Item).Name == msg.Version {
				updatedItem := it.(styles.Item)
				updatedItem.Installed = false
				items[i] = updatedItem
			}
		}
		m.List.SetItems(items)
		m.updateInstalledTable()
		m.Message = fmt.Sprintf("Successfully deleted Go %s", msg.Version)
		m.MessageType = "success"
		return m, nil
	}

	newListModel, cmd := m.List.Update(msg)
	m.List = newListModel
	cmds = append(cmds, cmd)
	newTableModel, tableCmd := m.InstalledTable.Update(msg)
	m.InstalledTable = newTableModel
	cmds = append(cmds, tableCmd)
	newDepsTableModel, depsTableCmd := m.Deps.Table.Update(msg)
	m.Deps.Table = newDepsTableModel
	cmds = append(cmds, depsTableCmd)
	return m, tea.Batch(cmds...)
}
