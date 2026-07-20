package model

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.Settings.EditingDepsBackupLimit {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			return m.handleDepsBackupLimitInputKey(key)
		}

		var cmd tea.Cmd
		m.Settings.DepsBackupLimitInput, cmd = m.Settings.DepsBackupLimitInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.Deps.Dialog.Active() {
			return m.handleDialogKey(msg)
		}
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		m.Layout = styles.GetLayoutMode(msg.Width)

		frameH, frameV := styles.FrameOverhead(m.Layout)
		contentWidth := msg.Width - frameH
		if contentWidth < 1 {
			contentWidth = 1
		}

		const fixedUIElements = 6
		contentHeight := msg.Height - frameV - fixedUIElements
		if contentHeight < 1 {
			contentHeight = 1
		}

		m.Width = contentWidth
		m.Height = contentHeight
		m.List.SetSize(contentWidth, contentHeight)
		m.InstalledTable.SetWidth(contentWidth)
		m.InstalledTable.SetHeight(contentHeight)
		m.InstalledTable.SetColumns(installedTableColumns(contentWidth))
		m.Deps.Table.SetWidth(contentWidth)
		m.Deps.Table.SetHeight(contentHeight)
		m.Deps.Table.SetColumns(dependencyTableColumns(contentWidth))
		return m, nil

	case utils.ErrMsg:
		m.Loading = false
		m.Status.SetGlobal(msg.Error(), "error")
		return m, nil

	case utils.VersionsMsg:
		m.Versions = msg
		m.Loading = false
		m.rebuildVersionViews()
		return m, nil

	case utils.DependenciesMsg:
		updateConfirmationOpen := m.Deps.Dialog.Kind == DialogUpdate
		m.Deps.Dependencies = msg
		m.Deps.Loaded = true
		m.Deps.Phase = OpIdle
		m.updateDependencyTable()
		if updateConfirmationOpen {
			m.resetDialog()
			m.Deps.UpdateEntries = nil
			m.Status.SetTab("Dependency updates changed. Please review them again.", "warning")
		} else {
			m.Status.ClearTab()
		}
		return m, nil

	case utils.DependencyBackupsMsg:
		m.Deps.Phase = OpIdle
		m.Deps.Backups = msg
		if len(msg) == 0 {
			m.Status.SetTab("No dependency backups found.", "warning")
			return m, nil
		}
		m.Deps.Dialog = ConfirmDialog{
			Kind:      DialogRestore,
			ChoiceYes: true,
			MaxCursor: len(msg) - 1,
		}
		m.Status.SetTab("Select a dependency backup to restore.", "info")
		return m, nil

	case utils.DependenciesUpdatedMsg:
		m.Deps.Phase = OpIdle
		m.Deps.Dependencies = msg.Dependencies
		m.Deps.Snapshot = msg.Snapshot
		m.Deps.LastCheckResult = nil
		m.updateDependencyTable()
		m.Status.SetGlobal(fmt.Sprintf("Updated %d direct %s. Run checks?", msg.Updated, utils.Pluralize(msg.Updated, "dependency", "dependencies")), "success")
		m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
		return m, nil

	case utils.DependencyCheckResultMsg:
		m.Deps.Phase = OpIdle
		m.resetDialog()
		if msg.OK {
			m.Status.SetGlobal("Checks passed.", "success")
			m.Deps.clearRollbackContext()
			return m, nil
		}
		m.Deps.LastCheckResult = &msg
		m.Deps.Dialog = ConfirmDialog{Kind: DialogRollback, ChoiceYes: true}
		m.Status.SetGlobal(fmt.Sprintf("Checks failed: %s", msg.Command), "error")
		return m, nil

	case utils.DependenciesRolledBackMsg:
		m.Deps.Phase = OpIdle
		m.Deps.Dependencies = msg.Dependencies
		m.Deps.clearRollbackContext()
		m.updateDependencyTable()
		m.Status.SetGlobal("Rolled back to pre-update state.", "success")
		return m, nil

	case utils.DependenciesRestoredMsg:
		m.Deps.Phase = OpIdle
		m.Deps.Dependencies = msg.Dependencies
		m.Deps.clearRollbackContext()
		m.updateDependencyTable()
		m.Status.SetGlobal(fmt.Sprintf("Restored dependencies from %s.", msg.BackupName), "success")
		return m, nil

	case utils.DependencyErrMsg:
		m.Deps.Reset()
		if msg.Err != nil {
			m.Status.SetGlobal(msg.Err.Error(), "error")
			return m, nil
		}
		m.Status.SetGlobal("", "error")
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
		m.rebuildVersionViews()
		m.Status.SetGlobal(fmt.Sprintf("Successfully installed Go %s", msg.Version), "success")
		return m, nil

	case utils.SwitchCompletedMsg:
		m.Loading = false
		for i := range m.Versions {
			m.Versions[i].Active = (m.Versions[i].Version == msg.Version)
		}
		m.rebuildVersionViews()
		if msg.ShimInPath {
			m.Status.SetGlobal(fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", msg.Version), "success")
		} else {
			m.Status.SetGlobal(fmt.Sprintf("Switched to Go %s!\n\n%s",
				msg.Version, utils.GetShimPathInstructions()), "success")
		}
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
		m.rebuildVersionViews()
		m.Status.SetGlobal(fmt.Sprintf("Successfully deleted Go %s", msg.Version), "success")
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
