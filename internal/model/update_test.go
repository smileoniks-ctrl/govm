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
	m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}
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

	if got.Deps.Dialog.Kind == DialogUpdate {
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

	if got.Deps.Phase == OpLoadingBackups {
		t.Fatal("expected LoadingBackups to be false after backups load")
	}
	if got.Deps.Dialog.Kind != DialogRestore {
		t.Fatal("expected restore dialog to open")
	}
	if got.Deps.Dialog.Cursor != 0 {
		t.Fatalf("expected backup cursor 0, got %d", got.Deps.Dialog.Cursor)
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

	if got.Deps.Phase == OpUpdating {
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

	if got.Deps.Phase == OpUpdating {
		t.Fatal("expected UpdatingDependencies to be false")
	}
	if got.Deps.Snapshot == nil {
		t.Fatal("expected LastDependencySnapshot to be set")
	}
	if got.Deps.Dialog.Kind != DialogChecks {
		t.Fatal("expected checks dialog to be open")
	}
	if !got.Deps.Dialog.ChoiceYes {
		t.Fatal("expected default choice to be Yes")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got %q", got.MessageType)
	}
}

func TestDependencyCheckResultOKClearsDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
	m.Deps.Snapshot = &utils.DependencySnapshot{}

	updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true})
	got := updated.(Model)

	if got.Deps.Dialog.Active() {
		t.Fatal("expected checks dialog to close after success")
	}
	if got.Deps.Phase == OpRunningChecks {
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
	m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
	m.Deps.Phase = OpRunningChecks

	msg := utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL",
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.Dialog.Kind == DialogChecks {
		t.Fatal("expected checks dialog to close on failure")
	}
	if got.Deps.Dialog.Kind != DialogRollback {
		t.Fatal("expected rollback dialog to be open")
	}
	if !got.Deps.Dialog.ChoiceYes {
		t.Fatal("expected default choice to be Yes")
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
	m.Deps.Phase = OpRollingBack
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

	if got.Deps.Phase == OpRollingBack {
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
	m.Deps.Phase = OpRollingBack

	updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("boom")})
	got := updated.(Model)

	if got.Deps.Phase == OpRollingBack {
		t.Fatal("expected RollingBackDependencies to be false after err")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
}

// newVersionCacheTestModel builds a Model with a multi-version catalog
// (installed, uninstalled, active) so that Download/Switch/Delete
// handlers have a non-trivial starting state to mutate. The list and
// table are populated via rebuildVersionViews so the model starts in a
// consistent state.
func newVersionCacheTestModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.Versions = []utils.GoVersion{
		{Version: "1.24.4", Filename: "go1.24.4.darwin-arm64.tar.gz", Installed: true, Active: true, Path: "/p/1.24.4"},
		{Version: "1.25.0", Filename: "go1.25.0.darwin-arm64.tar.gz", Installed: false},
		{Version: "1.26.0", Filename: "go1.26.0.darwin-arm64.tar.gz", Installed: true, Active: false, Path: "/p/1.26.0"},
	}
	m.rebuildVersionViews()
	return m
}

// TestVersionHandlersKeepCachesConsistent dispatches each of the four
// version-mutating msgs through Update and asserts the postcondition
// that the Available Versions list and the Installed Versions table
// remain exact projections of m.Versions afterwards.
func TestVersionHandlersKeepCachesConsistent(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{
			name: "VersionsMsg replaces catalog",
			msg: utils.VersionsMsg{
				{Version: "1.25.0", Filename: "go1.25.0.linux-amd64.tar.gz", Installed: true, Active: true, Path: "/p/1.25"},
				{Version: "1.27.0", Filename: "go1.27.0.linux-amd64.tar.gz"},
			},
		},
		{
			name: "DownloadCompleteMsg marks installed",
			msg:  utils.DownloadCompleteMsg{Version: "1.25.0", Path: "/new/1.25"},
		},
		{
			name: "SwitchCompletedMsg changes active",
			msg:  utils.SwitchCompletedMsg{Version: "1.26.0", ShimInPath: true},
		},
		{
			name: "DeleteCompleteMsg marks uninstalled",
			msg:  utils.DeleteCompleteMsg{Version: "1.24.4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newVersionCacheTestModel(t)
			updated, _ := m.Update(tt.msg)
			got := updated.(Model)
			assertVersionViewsConsistent(t, got)
		})
	}
}
