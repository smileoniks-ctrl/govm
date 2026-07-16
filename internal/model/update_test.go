package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestDependenciesMsgInvalidatesStaleUpdateConfirmation(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingUpdate = true
	m.Deps.Dependencies = []utils.ModuleDependency{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	m.Deps.UpdateEntries = []utils.DependencyUpdateEntry{
		{Path: "github.com/example/lib", OldVersion: "v1.0.0", NewVersion: "v1.1.0"},
	}

	incoming := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.1", Latest: "v1.1.0"},
	}
	updated, _ := m.Update(incoming)
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected stale update confirmation to close")
	}
	if len(got.Deps.UpdateEntries) != 0 {
		t.Fatalf("expected cached update entries to clear, got %d", len(got.Deps.UpdateEntries))
	}
	if !reflect.DeepEqual(got.Deps.Dependencies, []utils.ModuleDependency(incoming)) {
		t.Fatalf("dependencies = %+v, want %+v", got.Deps.Dependencies, incoming)
	}
	rows := got.Deps.Table.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 dependency table row, got %d", len(rows))
	}
	if rows[0][0] != incoming[0].Path ||
		rows[0][1] != incoming[0].Version ||
		rows[0][2] != incoming[0].Latest ||
		rows[0][3] != "update avail" {
		t.Fatalf("dependency table row = %q, want %q", rows[0], []string{
			incoming[0].Path,
			incoming[0].Version,
			incoming[0].Latest,
			"update avail",
		})
	}
	if !strings.Contains(strings.ToLower(got.Message), "review") ||
		!strings.Contains(strings.ToLower(got.Message), "again") {
		t.Fatalf("expected status to tell the user to review updates again, got %q", got.Message)
	}
}

func TestWindowSizeMsgKeepsContentSizesPositive(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	got := updated.(Model)

	if got.List.Width() <= 0 || got.List.Height() <= 0 {
		t.Fatalf("expected positive list size, got %dx%d", got.List.Width(), got.List.Height())
	}

	if got.InstalledTable.Width() <= 0 || got.InstalledTable.Height() <= 0 {
		t.Fatalf("expected positive table size, got %dx%d", got.InstalledTable.Width(), got.InstalledTable.Height())
	}
}

func TestWindowSizeMsgResizesDepsTable(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	got := updated.(Model)

	if got.Deps.Table.Width() <= 0 || got.Deps.Table.Height() <= 0 {
		t.Fatalf("expected positive deps table size, got %dx%d", got.Deps.Table.Width(), got.Deps.Table.Height())
	}
}

func TestWindowSizeMsgUsesNormalContentWidth(t *testing.T) {
	tests := []struct {
		name      string
		termWidth int
		wantWidth int
	}{
		{name: "minimum terminal", termWidth: 64, wantWidth: 60},
		{name: "normal terminal", termWidth: 80, wantWidth: 76},
		{name: "wide breakpoint", termWidth: 130, wantWidth: 124},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.termWidth, Height: 24})
			got := updated.(Model)

			if got.Width != tt.wantWidth {
				t.Fatalf("content width = %d, want %d", got.Width, tt.wantWidth)
			}
			if got.Deps.Table.Width() != tt.wantWidth {
				t.Fatalf("deps table width = %d, want %d", got.Deps.Table.Width(), tt.wantWidth)
			}
		})
	}
}

func TestDependencyBackupsMsgOpensRestoreDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(utils.DependencyBackupsMsg{
		{
			Name:       "2026-07-09_12-00-00.json",
			ModulePath: "github.com/acme/app",
			Kind:       utils.DependencyBackupKindPreUpdate,
			Updated:    1,
		},
	})
	got := updated.(Model)

	if got.Deps.LoadingBackups {
		t.Fatal("expected LoadingBackups to be false after backups load")
	}
	if !got.Deps.Dialog.ConfirmingRestoreBackup {
		t.Fatal("expected restore dialog to open")
	}
	if got.Deps.BackupCursor != 0 {
		t.Fatalf("expected backup cursor 0, got %d", got.Deps.BackupCursor)
	}
}

func TestDependenciesUpdatedMsgUpdatesState(t *testing.T) {
	m := newTestModel(t)

	msg := utils.DependenciesUpdatedMsg{
		Updated: 2,
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.1.0", Latest: "v1.1.0"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be false after update complete")
	}
	if len(got.Deps.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Deps.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got type %q", got.MessageType)
	}
}

func TestDependenciesUpdatedMsgStoresSnapshotAndOpensChecksDialog(t *testing.T) {
	m := newTestModel(t)

	msg := utils.DependenciesUpdatedMsg{
		Updated: 1,
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.1.0", Latest: "v1.1.0"},
		},
		Snapshot: &utils.DependencySnapshot{
			ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
			SumFile: utils.ModuleFileSnapshot{Exists: true, Content: "oldsum"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be false")
	}
	if got.Deps.Snapshot == nil {
		t.Fatal("expected LastDependencySnapshot to be set")
	}
	if !got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to be true")
	}
	if !got.Deps.Dialog.CheckChoiceYes {
		t.Fatal("expected CheckChoiceYes default to be Yes")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got %q", got.MessageType)
	}
}

func TestDependencyCheckResultOKClearsDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}

	updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close after success")
	}
	if got.Deps.RunningChecks {
		t.Fatal("expected RunningDependencyChecks to be false")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
	if got.Deps.Snapshot != nil {
		t.Fatal("expected Snapshot to be cleared after success")
	}
	if got.Deps.LastCheckResult != nil {
		t.Fatal("expected LastCheckResult to be cleared after success")
	}
}

func TestDependencyCheckResultFailOpensRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true
	m.Deps.RunningChecks = true

	msg := utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL",
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close on failure")
	}
	if !got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to be true")
	}
	if !got.Deps.Dialog.RollbackChoiceYes {
		t.Fatal("expected RollbackChoiceYes default to be Yes")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
	if got.Deps.LastCheckResult == nil || got.Deps.LastCheckResult.Command != "go test ./..." {
		t.Fatalf("expected LastCheckResult to capture failing command, got %+v", got.Deps.LastCheckResult)
	}
}

func TestDependenciesRolledBackMsgUpdatesState(t *testing.T) {
	m := newTestModel(t)
	m.Deps.RollingBack = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}

	msg := utils.DependenciesRolledBackMsg{
		Snapshot: &utils.DependencySnapshot{
			ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
		},
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to be false")
	}
	if len(got.Deps.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Deps.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
}

func TestDependencyErrDuringRollbackClearsState(t *testing.T) {
	m := newTestModel(t)
	m.Deps.RollingBack = true

	updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("boom")})
	got := updated.(Model)

	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to be false after err")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
}
