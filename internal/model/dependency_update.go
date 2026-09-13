package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

// resetDialog closes any open dependency confirmation dialog.
func (s *DepsState) resetDialog() {
	s.Dialog = ConfirmDialog{}
}

// setUpdatedDependencies applies the shared bookkeeping for standalone
// deps-result handlers (DependenciesMsg, DependenciesRestoredMsg):
// reset the standalone phase, store the dependency list, and rebuild
// the dependency table.
func (s *DepsState) setUpdatedDependencies(modules []deps.ModuleDependency) {
	s.Phase = OpIdle
	s.Dependencies = modules
	s.updateDependencyTable()
}

// handleDialogKey is the single entry point for key presses while a
// dependency confirmation dialog is open. Pure key handling (choice
// toggle, list navigation for restore) lives in ConfirmDialog.Handle;
// this method enacts the returned DialogAction by delegating to the
// per-kind apply* helpers. Quitting is the Model's business and never
// reaches here.
func (s *DepsState) handleDialogKey(msg tea.KeyPressMsg) (tea.Cmd, depsStatus) {
	newDialog, action := s.Dialog.Handle(msg)
	s.Dialog = newDialog
	switch action {
	case DialogConfirm:
		return s.applyDialogConfirm()
	case DialogCancel:
		return s.applyDialogCancel()
	case DialogChangeLevel:
		return s.applyDialogLevelChange()
	case DialogChangeScope:
		return s.applyDialogScopeChange()
	}
	return nil, depsStatus{}
}

// applyDialogLevelChange asks the Cycle to rebuild the plan at the
// level the dialog now shows. The re-emitted IntentConfirmApply
// replaces the dialog contents; the dialog stays open.
func (s *DepsState) applyDialogLevelChange() (tea.Cmd, depsStatus) {
	if s.Dialog.Kind != DialogUpdate {
		return nil, depsStatus{}
	}
	return s.rebuildDialogPlan(deps.ChangeLevelEvent{Level: s.Dialog.Level})
}

// applyDialogScopeChange asks the Cycle to rebuild the plan for the
// Update scope the dialog now shows: the explicit module set, or
// every direct dependency.
func (s *DepsState) applyDialogScopeChange() (tea.Cmd, depsStatus) {
	if s.Dialog.Kind != DialogUpdate {
		return nil, depsStatus{}
	}
	var modules []string
	if s.Dialog.Explicit {
		modules = s.Dialog.ExplicitModules
	}
	return s.rebuildDialogPlan(deps.ChangeScopeEvent{Modules: modules})
}

// rebuildDialogPlan feeds a plan-changing event into the Cycle while
// the update dialog is open. The re-emitted IntentConfirmApply
// replaces the dialog contents; the dialog stays open.
func (s *DepsState) rebuildDialogPlan(event deps.Event) (tea.Cmd, depsStatus) {
	next, intent, err := s.Cycle.Handle(event)
	if err != nil {
		s.Cycle = deps.NewUpdateCycle()
		s.resetDialog()
		return nil, depsGlobalStatus(err.Error(), "error")
	}
	s.Cycle = next
	return s.applyCycleIntent(intent)
}

// applyDialogConfirm dispatches the confirm action. Update/checks/
// rollback confirmations feed the user's choice to the Cycle as a
// decision event; restore runs its own standalone command.
func (s *DepsState) applyDialogConfirm() (tea.Cmd, depsStatus) {
	switch s.Dialog.Kind {
	case DialogUpdate:
		return s.feedCycleDecision(deps.ConfirmApplyEvent{Yes: s.Dialog.ChoiceYes})
	case DialogChecks:
		return s.feedCycleDecision(deps.ConfirmChecksEvent{Yes: s.Dialog.ChoiceYes})
	case DialogRollback:
		return s.feedCycleDecision(deps.ConfirmRollbackEvent{Yes: s.Dialog.ChoiceYes})
	case DialogRestore:
		return s.applyRestoreBackupChoice()
	}
	s.resetDialog()
	return nil, depsStatus{}
}

// applyDialogCancel dispatches the cancel action. Update/checks/
// rollback cancel feed a No decision to the Cycle; restore is torn
// down locally.
func (s *DepsState) applyDialogCancel() (tea.Cmd, depsStatus) {
	switch s.Dialog.Kind {
	case DialogUpdate:
		return s.feedCycleDecision(deps.ConfirmApplyEvent{Yes: false})
	case DialogChecks:
		return s.feedCycleDecision(deps.ConfirmChecksEvent{Yes: false})
	case DialogRollback:
		return s.feedCycleDecision(deps.ConfirmRollbackEvent{Yes: false})
	case DialogRestore:
		s.resetDialog()
		return nil, depsTabStatus("Restore canceled.", "info")
	}
	s.resetDialog()
	return nil, depsStatus{}
}

// feedCycleDecision closes the dialog and feeds a decision event into
// the Cycle through the central adapter.
func (s *DepsState) feedCycleDecision(event deps.Event) (tea.Cmd, depsStatus) {
	s.resetDialog()
	return s.handleCycleEvent(event)
}

func (s *DepsState) applyRestoreBackupChoice() (tea.Cmd, depsStatus) {
	if !s.Dialog.ChoiceYes {
		s.resetDialog()
		return nil, depsTabStatus("Restore canceled.", "info")
	}
	if len(s.Backups) == 0 || s.Dialog.Cursor < 0 || s.Dialog.Cursor >= len(s.Backups) {
		s.resetDialog()
		return nil, depsTabStatus("Restore unavailable: no backup selected.", "error")
	}

	backup := s.Backups[s.Dialog.Cursor]
	s.resetDialog()
	s.Phase = OpRestoringBackup
	return RestoreDependencyBackupCmd(s.executor(), backup.Name),
		depsGlobalStatus("Restoring dependency backup...", "info")
}
