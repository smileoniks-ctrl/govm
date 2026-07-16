package model

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestResetUpdateConfirmationClearsDialogAndEntries(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingUpdate = true
	m.Deps.Dialog.UpdateChoiceYes = true
	m.Deps.UpdateEntries = []utils.DependencyUpdateEntry{{Path: "example.com/dependency"}}

	m.resetUpdateConfirmation()

	if m.Deps.Dialog.ConfirmingUpdate || m.Deps.Dialog.UpdateChoiceYes {
		t.Fatal("expected update dialog state to reset")
	}
	if !reflect.DeepEqual(m.Deps.UpdateEntries, []utils.DependencyUpdateEntry(nil)) {
		t.Fatalf("update entries = %#v, want nil", m.Deps.UpdateEntries)
	}
}

func TestResetChecksConfirmationClearsDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true

	m.resetChecksConfirmation()

	if m.Deps.Dialog.ConfirmingChecks || m.Deps.Dialog.CheckChoiceYes {
		t.Fatal("expected checks dialog state to reset")
	}
}

func TestRestoreBackupDialogEnterTriggersRestoreCmd(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRestoreBackup = true
	m.Deps.Dialog.RestoreChoiceYes = true
	m.Deps.Backups = []utils.DependencyBackupInfo{
		{Name: "2026-07-09_12-00-00.json"},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRestoreBackup {
		t.Fatal("expected restore dialog to close")
	}
	if !got.Deps.RestoringBackup {
		t.Fatal("expected RestoringBackup to be true")
	}
	if cmd == nil {
		t.Fatal("expected restore command")
	}
}

func TestEscClosesConfirmDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	if len(m.Deps.UpdateEntries) != 1 {
		t.Fatalf("expected 1 cached update entry, got %d", len(m.Deps.UpdateEntries))
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected dialog to close on esc")
	}
	if len(m.Deps.UpdateEntries) != 0 {
		t.Fatalf("expected cached update entries to clear on cancel, got %d", len(m.Deps.UpdateEntries))
	}
}

func TestRightArrowTogglesDialogChoice(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	if !m.Deps.Dialog.UpdateChoiceYes {
		t.Fatal("expected default to be Yes")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(Model)

	if m.Deps.Dialog.UpdateChoiceYes {
		t.Fatal("expected right arrow to toggle choice to No")
	}
}

func TestConfirmOnNoClosesDialogWithoutUpdate(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // toggle to No
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected dialog to close after confirm on No")
	}
	if m.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be false after choosing No")
	}
}

func TestConfirmOnYesTriggersUpdateCmd(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected dialog to close after confirm on Yes")
	}
	if !m.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be true after choosing Yes")
	}
	if len(m.Deps.UpdateEntries) != 0 {
		t.Fatalf("expected cached update entries to clear on confirm, got %d", len(m.Deps.UpdateEntries))
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned after confirming Yes")
	}
}

func TestRollbackCmdTriggeredByRollbackYes(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{
		ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
		SumFile: utils.ModuleFileSnapshot{Exists: true, Content: "oldsum"},
	}
	m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test ./..."}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close")
	}
	if !got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to be true")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned for rollback")
	}
}

func TestKeepCmdClearsRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}
	m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test"}

	// Toggle to No then confirm.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close when keeping updates")
	}
	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
	if got.MessageType != "warning" {
		t.Fatalf("expected warning status, got %q", got.MessageType)
	}
}

func TestEscOnChecksDialogSkipsChecks(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.RunningChecks {
		t.Fatal("expected RunningDependencyChecks to remain false")
	}
}

func TestEscOnRollbackDialogKeepsUpdates(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
}
