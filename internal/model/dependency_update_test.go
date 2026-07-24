package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

func TestResetUpdateConfirmationClearsDialogAndEntries(t *testing.T) {
	m := modelAtConfirmApply(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected update dialog to close on cancel")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if len(got.Deps.Cycle.Entries()) != 0 {
		t.Fatalf("cycle entries = %d, want 0", len(got.Deps.Cycle.Entries()))
	}
}

func TestResetChecksConfirmationClearsDialog(t *testing.T) {
	m := modelAtConfirmChecks(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected checks dialog to close on cancel")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
}

func TestQuitKeysWorkWhileDialogOpen(t *testing.T) {
	keys := []tea.KeyPressMsg{
		{Code: 'q'},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	for _, key := range keys {
		m := modelAtConfirmApply(t)
		_, cmd := m.Update(key)
		if cmd == nil || cmd() == nil {
			t.Fatalf("key %q did not return a quit command", key.String())
		}
	}
}

func TestRestoreBackupDialogEnterTriggersRestoreCmd(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Backups = []deps.DependencyBackupInfo{
		{Name: "2026-07-09_12-00-00.json"},
	}
	m.Deps.Dialog = ConfirmDialog{
		Kind:      DialogRestore,
		ChoiceYes: true,
		MaxCursor: len(m.Deps.Backups) - 1,
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected restore dialog to close")
	}
	if got.Deps.Phase != OpRestoringBackup {
		t.Fatal("expected RestoringBackup to be true")
	}
	if cmd == nil {
		t.Fatal("expected restore command")
	}
}

func TestEscClosesConfirmDialog(t *testing.T) {
	m := modelAtConfirmApply(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if m.Deps.Dialog.Active() {
		t.Fatal("expected dialog to close on esc")
	}
	if len(m.Deps.Cycle.Entries()) != 0 {
		t.Fatalf("expected cycle entries to clear on cancel, got %d", len(m.Deps.Cycle.Entries()))
	}
}

func TestRightArrowTogglesDialogChoice(t *testing.T) {
	m := modelAtConfirmApply(t)

	if !m.Deps.Dialog.ChoiceYes {
		t.Fatal("expected default to be Yes")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(Model)

	if m.Deps.Dialog.ChoiceYes {
		t.Fatal("expected right arrow to toggle choice to No")
	}
}

func TestConfirmOnNoClosesDialogWithoutUpdate(t *testing.T) {
	m := modelAtConfirmApply(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Deps.Dialog.Active() {
		t.Fatal("expected dialog to close after confirm on No")
	}
	if m.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", m.Deps.Cycle.Phase())
	}
}

func TestConfirmOnYesTriggersUpdateCmd(t *testing.T) {
	m := modelAtConfirmApply(t)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Deps.Dialog.Active() {
		t.Fatal("expected dialog to close after confirm on Yes")
	}
	if m.Deps.Cycle.Phase() != deps.PhaseApplying {
		t.Fatalf("cycle phase = %s, want applying", m.Deps.Cycle.Phase())
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned after confirming Yes")
	}
}

func TestRollbackCmdTriggeredByRollbackYes(t *testing.T) {
	m := modelAtConfirmRollback(t)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected rollback dialog to close")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseRollingBack {
		t.Fatalf("cycle phase = %s, want rolling-back", got.Deps.Cycle.Phase())
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned for rollback")
	}
}

func TestKeepCmdClearsRollbackDialog(t *testing.T) {
	m := modelAtConfirmRollback(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected rollback dialog to close when keeping updates")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if got.Status.Kind() != "warning" {
		t.Fatalf("expected warning status, got %q", got.Status.Kind())
	}
}

func TestEscOnChecksDialogSkipsChecks(t *testing.T) {
	m := modelAtConfirmChecks(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
}

func TestEscOnRollbackDialogKeepsUpdates(t *testing.T) {
	m := modelAtConfirmRollback(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
}

func modelAtConfirmApply(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	cycle := mustCycleEvent(t, deps.NewUpdateCycle(), deps.StartEvent{ModuleDir: "/tmp/module"})
	cycle = mustCycleEvent(t, cycle, deps.CheckUpdatesDoneEvent{Dependencies: []deps.ModuleDependency{{
		Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0",
	}}})
	m.Deps.Cycle = cycle
	m.Deps.Dialog = ConfirmDialog{
		Kind:          DialogUpdate,
		ChoiceYes:     true,
		UpdateEntries: cycle.Entries(),
	}
	return m
}

func modelAtConfirmChecks(t *testing.T) Model {
	t.Helper()
	m := modelAtConfirmApply(t)
	cycle := mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmApplyEvent{Yes: true})
	cycle = mustCycleEvent(t, cycle, deps.ApplyUpdatesDoneEvent{
		Snapshot: &deps.DependencySnapshot{
			ModFile: deps.ModuleFileSnapshot{Exists: true, Content: "module example.com/app\n"},
		},
		Backup:       &deps.DependencyBackupInfo{Name: "backup.json", Path: "/tmp/backup.json"},
		Dependencies: m.Deps.Cycle.Dependencies(),
	})
	m.Deps.Cycle = cycle
	m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
	return m
}

func modelAtConfirmRollback(t *testing.T) Model {
	t.Helper()
	m := modelAtConfirmChecks(t)
	cycle := mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmChecksEvent{Yes: true})
	cycle = mustCycleEvent(t, cycle, deps.ChecksDoneEvent{
		Result: deps.DependencyCheckResult{Command: "go test ./...", Output: "FAIL"},
	})
	m.Deps.Cycle = cycle
	m.Deps.Dialog = ConfirmDialog{
		Kind:        DialogRollback,
		ChoiceYes:   true,
		CheckResult: cycle.CheckResult(),
	}
	return m
}
