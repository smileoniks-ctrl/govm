package model

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
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
		m.list.SetSize(contentWidth, contentHeight)
		m.installedTable.SetWidth(contentWidth)
		m.installedTable.SetHeight(contentHeight)
		m.installedTable.SetColumns(installedTableColumns(contentWidth))
		m.Deps.Table.SetWidth(contentWidth)
		m.Deps.Table.SetHeight(contentHeight)
		m.Deps.Table.SetColumns(dependencyTableColumns(contentWidth))
		return m, nil

	case utils.ErrMsg:
		m.Loading = false
		// A fetch failure while reconciling must drop the pending
		// context so a later normal VersionsMsg is not misread as the
		// verification of an operation that never got re-checked.
		m.reconcile = reconcileContext{}
		m.Status.SetGlobal(msg.Error(), "error")
		return m, nil

	case utils.VersionsMsg:
		return m.handleVersionsMsg(msg)

	case list.FilterMatchesMsg:
		// Projection-triggered refilters are wrapped when the command is
		// created. A plain FilterMatchesMsg belongs to normal filtering
		// and is applied directly exactly once.
		newList, cmd := m.list.Update(msg)
		m.list = newList
		return m, cmd

	case refilterSettledMsg:
		return m.handleRefilterSettled(msg)

	case DependenciesMsg:
		m.Deps.Dependencies = msg
		m.Deps.Loaded = true
		m.Deps.Phase = OpIdle
		m.updateDependencyTable()
		m.Status.ClearTab()
		return m, nil

	case DependencyBackupsMsg:
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

	case deps.CheckUpdatesDoneEvent,
		deps.ApplyUpdatesDoneEvent,
		deps.CompensateDoneEvent,
		deps.ChecksDoneEvent,
		deps.RollbackDoneEvent:
		return m.handleCycleEvent(msg.(deps.Event))

	case DependenciesRestoredMsg:
		m.setUpdatedDependencies(msg.Dependencies)
		m.Status.SetGlobal(fmt.Sprintf("Restored dependencies from %s.", msg.BackupName), "success")
		return m, nil

	case dependencyExecutionErrMsg:
		m.Deps.Cycle = deps.NewUpdateCycle()
		m.resetDialog()
		m.Status.SetGlobal(msg.Err.Error(), "error")
		return m, nil

	case DependencyErrMsg:
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

	case installSuccessMsg:
		return m.handleInstallSuccess(msg)

	case installFailureMsg:
		return m.handleInstallFailure(msg)

	case utils.SwitchCompletedMsg:
		return m.handleSwitchCompleted(msg)

	case utils.DeleteCompleteMsg:
		return m.handleDeleteComplete(msg)
	}

	newListModel, cmd := m.list.Update(msg)
	m.list = newListModel
	cmds = append(cmds, cmd)
	newTableModel, tableCmd := m.installedTable.Update(msg)
	m.installedTable = newTableModel
	cmds = append(cmds, tableCmd)
	newDepsTableModel, depsTableCmd := m.Deps.Table.Update(msg)
	m.Deps.Table = newDepsTableModel
	cmds = append(cmds, depsTableCmd)
	return m, tea.Batch(cmds...)
}

// handleVersionsMsg applies a fetched catalog. When a reconciliation is
// pending (a completion mutation reported a catalog error), the fresh
// snapshot is used to verify the expected disk side-effect; otherwise
// it is a normal load. A normal VersionsMsg never fabricates a success
// status.
func (m Model) handleVersionsMsg(msg utils.VersionsMsg) (tea.Model, tea.Cmd) {
	if m.reconcile.active {
		cmd, err := m.replaceVersions(msg)
		if err != nil {
			// Invalid reconciliation snapshot: stop loading, keep the
			// previous catalog, surface the error and clear the pending
			// context to avoid a reconcile loop.
			m.Loading = false
			m.reconcile = reconcileContext{}
			m.Status.SetGlobal(fmt.Sprintf("Could not verify the operation: %v.", err), "error")
			return m, nil
		}
		pending := m.reconcile
		confirmed := m.verifyReconciliation()
		m.reconcile = reconcileContext{}
		if confirmed {
			m.applyReconciliationSuccess(pending)
			return m, cmd
		}
		m.Loading = false
		m.Status.SetGlobal("The operation could not be confirmed against the installed catalog.", "error")
		return m, cmd
	}

	cmd, err := m.replaceVersions(msg)
	if err != nil {
		// Invalid snapshot: keep the previous catalog, stop loading and
		// surface the error.
		m.Loading = false
		m.Status.SetGlobal(fmt.Sprintf("Failed to load Go versions: %v.", err), "error")
		return m, nil
	}
	m.Loading = false
	return m, cmd
}

// handleRefilterSettled applies a deferred refilter result. Stale
// results (superseded by a newer projection or further filter input)
// are dropped; otherwise the matches are applied to the list exactly
// once and, when this refilter belongs to a pending projection, the
// captured Available-list selection is restored by identity.
func (m Model) handleRefilterSettled(msg refilterSettledMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.projectionGeneration || msg.filterText != m.list.FilterInput.Value() {
		if m.pendingListRestore.active &&
			m.pendingListRestore.generation == msg.generation &&
			m.pendingListRestore.filterText == msg.filterText {
			m.pendingListRestore = pendingListRestore{}
		}
		return m, nil
	}
	newList, _ := m.list.Update(msg.matches)
	m.list = newList
	if m.pendingListRestore.active &&
		m.pendingListRestore.generation == msg.generation &&
		m.pendingListRestore.filterText == msg.filterText {
		m.restoreListSelection(m.pendingListRestore.version, m.pendingListRestore.index)
		m.pendingListRestore = pendingListRestore{}
	}
	return m, nil
}

// handleInstallSuccess marks the version installed via the catalog.
// On a catalog error the disk install may still have succeeded, so the
// model re-fetches and reconciles instead of trusting the stale state.
// Result warnings are copied into the reconcile context so they survive
// a successful reconciliation and surface in the final status.
func (m Model) handleInstallSuccess(msg installSuccessMsg) (tea.Model, tea.Cmd) {
	m.InstallingVersion = ""
	_, cmd, err := m.markVersionInstalled(msg.Version, msg.Path)
	if err != nil {
		m.Loading = true
		m.reconcile = reconcileContext{
			active:    true,
			operation: reconcileInstall,
			version:   msg.Version,
			warnings:  msg.Warnings,
		}
		m.Status.SetGlobal(fmt.Sprintf("Installed Go %s; verifying catalog...", msg.Version), "warning")
		return m, tea.Batch(cmd, utils.FetchGoVersions)
	}
	m.Loading = false
	text, kind := installSuccessStatus(msg.Version, msg.Warnings)
	m.Status.SetGlobal(text, kind)
	return m, cmd
}

// handleInstallFailure clears the install/loading state and reports the
// phase-aware error returned by the install core. Any pending
// reconciliation is dropped because the disk operation did not succeed.
func (m Model) handleInstallFailure(msg installFailureMsg) (tea.Model, tea.Cmd) {
	m.Loading = false
	m.InstallingVersion = ""
	m.reconcile = reconcileContext{}
	m.Status.SetGlobal(fmt.Sprintf("Failed to install Go %s: %v", msg.Version, msg.Err), "error")
	return m, nil
}

// handleSwitchCompleted activates the version via the catalog, or
// reconciles on a catalog error. The success status stays tab-scoped
// to mirror the direct completion path.
func (m Model) handleSwitchCompleted(msg utils.SwitchCompletedMsg) (tea.Model, tea.Cmd) {
	_, cmd, err := m.activateVersion(msg.Version)
	if err != nil {
		m.Loading = true
		m.reconcile = reconcileContext{active: true, operation: reconcileSwitch, version: msg.Version, shimInPath: msg.ShimInPath}
		m.Status.SetGlobal(fmt.Sprintf("Switched to Go %s; verifying catalog...", msg.Version), "warning")
		return m, tea.Batch(cmd, utils.FetchGoVersions)
	}
	m.Loading = false
	if msg.ShimInPath {
		m.Status.SetTab(fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", msg.Version), "success")
	} else {
		m.Status.SetTab(fmt.Sprintf("Switched to Go %s!\n\n%s", msg.Version, utils.GetShimPathInstructions()), "success")
	}
	return m, cmd
}

// handleDeleteComplete marks the version deleted via the catalog, or
// reconciles on a catalog error.
func (m Model) handleDeleteComplete(msg utils.DeleteCompleteMsg) (tea.Model, tea.Cmd) {
	_, cmd, err := m.markVersionDeleted(msg.Version)
	if err != nil {
		m.Loading = true
		m.reconcile = reconcileContext{active: true, operation: reconcileDelete, version: msg.Version}
		m.Status.SetGlobal(fmt.Sprintf("Deleted Go %s; verifying catalog...", msg.Version), "warning")
		return m, tea.Batch(cmd, utils.FetchGoVersions)
	}
	m.Loading = false
	m.Status.SetGlobal(fmt.Sprintf("Successfully deleted Go %s", msg.Version), "success")
	return m, cmd
}
