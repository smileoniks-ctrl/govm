package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// resetDialog closes any open dependency confirmation dialog. Per-kind
// teardown (clearing UpdateEntries, rolling back context, etc.) lives
// in the apply* and cancel* helpers below, which call this after they
// have finished their kind-specific work.
func (m *Model) resetDialog() {
	m.Deps.Dialog = ConfirmDialog{}
}

// setUpdatedDependencies applies the shared bookkeeping spine common
// to every deps-result handler (DependenciesUpdatedMsg,
// DependenciesRolledBackMsg, DependenciesRestoredMsg): reset the
// operation phase, store the freshly delivered dependency list, and
// rebuild the dependency table. Per-msg divergent tails (snapshot
// retention, dialog open, rollback-context teardown) stay inline in
// the handlers. Full unification of the workflow is deferred to the
// proposed Dep Update Cycle module.
func (m *Model) setUpdatedDependencies(deps []utils.ModuleDependency) {
	m.Deps.Phase = OpIdle
	m.Deps.Dependencies = deps
	m.updateDependencyTable()
}

// handleDialogKey is the single entry point for key presses while a
// dependency confirmation dialog is open. Pure key handling (choice
// toggle, list navigation for restore) lives in ConfirmDialog.Handle;
// this method enacts the returned DialogAction by delegating to the
// per-kind apply*Choice helpers for confirm, or by running the
// per-kind cancel path for cancel.
func (m Model) handleDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

// applyDialogConfirm dispatches the confirm action to the per-kind
// side-effect runner. Each runner returns the tea.Cmd that actually
// performs the work (UpdateModuleDependencies, RunModuleDependencyChecks,
// RollbackModuleDependencies, RestoreDependencyBackup).
func (m Model) applyDialogConfirm() (tea.Model, tea.Cmd) {
	switch m.Deps.Dialog.Kind {
	case DialogUpdate:
		return m.applyUpdateChoice()
	case DialogChecks:
		return m.applyChecksChoice()
	case DialogRollback:
		return m.applyRollbackChoice()
	case DialogRestore:
		return m.applyRestoreBackupChoice()
	}
	m.resetDialog()
	return m, nil
}

// applyDialogCancel dispatches the cancel action to per-kind teardown.
// Each kind has its own message and rollback-context policy.
func (m Model) applyDialogCancel() (tea.Model, tea.Cmd) {
	switch m.Deps.Dialog.Kind {
	case DialogUpdate:
		m.resetDialog()
		m.Deps.UpdateEntries = nil
		m.Status.SetTab("Update canceled.", "info")
		return m, nil
	case DialogChecks:
		m.resetDialog()
		m.Deps.clearRollbackContext()
		m.Status.SetGlobal("Update complete. Checks skipped.", "info")
		return m, nil
	case DialogRollback:
		return m.keepUpdatedDependencies()
	case DialogRestore:
		m.resetDialog()
		m.Status.SetTab("Restore canceled.", "info")
		return m, nil
	}
	m.resetDialog()
	return m, nil
}

func (m Model) applyUpdateChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.ChoiceYes {
		m.resetDialog()
		m.Deps.UpdateEntries = nil
		m.Status.SetTab("Update canceled.", "info")
		return m, nil
	}

	entries := m.Deps.UpdateEntries
	m.resetDialog()
	m.Deps.UpdateEntries = nil
	m.Deps.Phase = OpUpdating
	m.Status.SetGlobal("Updating dependencies...", "info")
	return m, utils.UpdateModuleDependencies(m.Deps.ModuleDir, entries, m.Settings.Values.DepsBackupLimit)
}

func (m Model) applyChecksChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.ChoiceYes {
		m.resetDialog()
		m.Deps.clearRollbackContext()
		m.Status.SetGlobal("Update complete. Checks skipped.", "info")
		return m, nil
	}

	m.resetDialog()
	m.Deps.Phase = OpRunningChecks
	m.Status.SetGlobal("Running checks...", "info")
	return m, utils.RunModuleDependencyChecks(m.Deps.ModuleDir)
}

func (m Model) applyRollbackChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.ChoiceYes {
		return m.keepUpdatedDependencies()
	}

	if m.Deps.Snapshot == nil {
		m.resetDialog()
		m.Status.SetTab("Rollback unavailable: snapshot is missing.", "error")
		return m, nil
	}

	snap := m.Deps.Snapshot
	m.resetDialog()
	m.Deps.Phase = OpRollingBack
	m.Status.SetGlobal("Rolling back dependencies...", "info")
	return m, utils.RollbackModuleDependencies(m.Deps.ModuleDir, snap)
}

func (m Model) keepUpdatedDependencies() (tea.Model, tea.Cmd) {
	m.resetDialog()
	m.Deps.clearRollbackContext()
	m.Status.SetGlobal("Update kept. Failed checks were not rolled back.", "warning")
	return m, nil
}

func (m Model) applyRestoreBackupChoice() (tea.Model, tea.Cmd) {
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
	return m, utils.RestoreDependencyBackup(m.Deps.ModuleDir, backup.Name, m.Settings.Values.DepsBackupLimit)
}
