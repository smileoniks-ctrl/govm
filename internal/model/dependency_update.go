package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

// resetDialog closes any open dependency confirmation dialog.
func (m *Model) resetDialog() {
	m.Deps.Dialog = ConfirmDialog{}
}

// setUpdatedDependencies applies the shared bookkeeping for standalone
// deps-result handlers (DependenciesMsg, DependenciesRestoredMsg):
// reset the standalone phase, store the dependency list, and rebuild
// the dependency table.
func (m *Model) setUpdatedDependencies(modules []deps.ModuleDependency) {
	m.Deps.Phase = OpIdle
	m.Deps.Dependencies = modules
	m.updateDependencyTable()
}

// handleDialogKey is the single entry point for key presses while a
// dependency confirmation dialog is open. Pure key handling (choice
// toggle, list navigation for restore) lives in ConfirmDialog.Handle;
// this method enacts the returned DialogAction by delegating to the
// per-kind apply* helpers.
func (m *Model) handleDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	newDialog, action := m.Deps.Dialog.Handle(msg)
	m.Deps.Dialog = newDialog
	switch action {
	case DialogConfirm:
		return m.applyDialogConfirm()
	case DialogCancel:
		return m.applyDialogCancel()
	}
	return m, nil
}

// applyDialogConfirm dispatches the confirm action. Update/checks/
// rollback confirmations feed the user's choice to the Cycle as a
// decision event; restore runs its own standalone command.
func (m *Model) applyDialogConfirm() (tea.Model, tea.Cmd) {
	switch m.Deps.Dialog.Kind {
	case DialogUpdate:
		return m.feedCycleDecision(deps.ConfirmApplyEvent{Yes: m.Deps.Dialog.ChoiceYes})
	case DialogChecks:
		return m.feedCycleDecision(deps.ConfirmChecksEvent{Yes: m.Deps.Dialog.ChoiceYes})
	case DialogRollback:
		return m.feedCycleDecision(deps.ConfirmRollbackEvent{Yes: m.Deps.Dialog.ChoiceYes})
	case DialogRestore:
		return m.applyRestoreBackupChoice()
	}
	m.resetDialog()
	return m, nil
}

// applyDialogCancel dispatches the cancel action. Update/checks/
// rollback cancel feed a No decision to the Cycle; restore is torn
// down locally.
func (m *Model) applyDialogCancel() (tea.Model, tea.Cmd) {
	switch m.Deps.Dialog.Kind {
	case DialogUpdate:
		return m.feedCycleDecision(deps.ConfirmApplyEvent{Yes: false})
	case DialogChecks:
		return m.feedCycleDecision(deps.ConfirmChecksEvent{Yes: false})
	case DialogRollback:
		return m.feedCycleDecision(deps.ConfirmRollbackEvent{Yes: false})
	case DialogRestore:
		m.resetDialog()
		m.Status.SetTab("Restore canceled.", "info")
		return m, nil
	}
	m.resetDialog()
	return m, nil
}

// feedCycleDecision closes the dialog and feeds a decision event into
// the Cycle through the central adapter.
func (m *Model) feedCycleDecision(event deps.Event) (tea.Model, tea.Cmd) {
	m.resetDialog()
	return m.handleCycleEvent(event)
}

func (m *Model) applyRestoreBackupChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.ChoiceYes {
		m.resetDialog()
		m.Status.SetTab("Restore canceled.", "info")
		return m, nil
	}
	if len(m.Deps.Backups) == 0 || m.Deps.Dialog.Cursor < 0 || m.Deps.Dialog.Cursor >= len(m.Deps.Backups) {
		m.resetDialog()
		m.Status.SetTab("Restore unavailable: no backup selected.", "error")
		return m, nil
	}

	backup := m.Deps.Backups[m.Deps.Dialog.Cursor]
	m.resetDialog()
	m.Deps.Phase = OpRestoringBackup
	m.Status.SetGlobal("Restoring dependency backup...", "info")
	return m, RestoreDependencyBackupCmd(
		m.Deps.ModuleDir,
		backup.Name,
		m.Settings.Values.DepsBackupLimit,
	)
}
