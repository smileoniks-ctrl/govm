package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestUpdateKeyStartsFreshPreflight(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Loaded = true
	m.Deps.Dependencies = []deps.ModuleDependency{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	var issued deps.Intent
	m.Deps.ExecuteIntent = func(intent deps.Intent) tea.Cmd {
		issued = intent
		return func() tea.Msg { return nil }
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'u'})
	got := updated.(Model)

	if cmd == nil {
		t.Fatal("expected fresh check command")
	}
	if got.Deps.Cycle.Phase() != deps.PhaseChecking {
		t.Fatalf("cycle phase = %s, want checking", got.Deps.Cycle.Phase())
	}
	if _, ok := issued.(deps.IntentCheckUpdates); !ok {
		t.Fatalf("issued intent = %T, want IntentCheckUpdates", issued)
	}
	if got.Deps.Dialog.Active() {
		t.Fatal("update dialog must wait for the fresh preflight result")
	}
}

func TestFreshPreflightResultOpensUpdateDialog(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Loaded = true
	m.Deps.Cycle = mustCycleEvent(t, deps.NewUpdateCycle(), deps.StartEvent{ModuleDir: "/tmp/module"})

	fresh := []deps.ModuleDependency{
		{Path: "github.com/example/lib", Version: "v1.0.1", Latest: "v1.2.0"},
	}
	updated, _ := m.Update(deps.CheckUpdatesDoneEvent{Dependencies: fresh})
	got := updated.(Model)

	if got.Deps.Dialog.Kind != DialogUpdate {
		t.Fatalf("dialog kind = %v, want DialogUpdate", got.Deps.Dialog.Kind)
	}
	entries := got.Deps.Cycle.Entries()
	if len(entries) != 1 || entries[0].OldVersion != "v1.0.1" || entries[0].NewVersion != "v1.2.0" {
		t.Fatalf("fresh entries = %+v", entries)
	}
	if !reflect.DeepEqual(got.Deps.Cycle.Dependencies(), fresh) {
		t.Fatalf("dependencies = %+v, want %+v", got.Deps.Cycle.Dependencies(), fresh)
	}
	if !got.Deps.Dialog.ChoiceYes {
		t.Fatal("expected default choice Yes")
	}
	if !reflect.DeepEqual(got.Deps.Dialog.UpdateEntries, entries) {
		t.Fatalf("dialog entries = %+v, want %+v", got.Deps.Dialog.UpdateEntries, entries)
	}
}

func TestUnknownCycleIntentReportsError(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Cycle = mustCycleEvent(t, deps.NewUpdateCycle(), deps.StartEvent{ModuleDir: "/tmp/module"})

	updated, _ := m.applyCycleIntent(nil)
	got := updated.(Model)
	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if got.Status.Kind() != "error" || !strings.Contains(got.Status.Text(), "Unhandled dependency cycle intent") {
		t.Fatalf("status = %q (%s)", got.Status.Text(), got.Status.Kind())
	}
}

func TestWindowSizeMsgKeepsContentSizesPositive(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	got := updated.(Model)

	if got.list.Width() <= 0 || got.list.Height() <= 0 {
		t.Fatalf("expected positive list size, got %dx%d", got.list.Width(), got.list.Height())
	}

	if got.installedTable.Width() <= 0 || got.installedTable.Height() <= 0 {
		t.Fatalf("expected positive table size, got %dx%d", got.installedTable.Width(), got.installedTable.Height())
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

	updated, _ := m.Update(DependencyBackupsMsg{
		{
			Name:       "2026-07-09_12-00-00.json",
			ModulePath: "github.com/acme/app",
			Kind:       deps.DependencyBackupKindPreUpdate,
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

func TestApplyResultUpdatesStateAndOpensChecksDialog(t *testing.T) {
	m := modelAtConfirmApply(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmApplyEvent{Yes: true})
	dependencies := []deps.ModuleDependency{{
		Path: "github.com/example/lib", Version: "v1.1.0", Latest: "v1.1.0",
	}}

	updated, _ := m.Update(deps.ApplyUpdatesDoneEvent{
		Snapshot: &deps.DependencySnapshot{
			ModFile: deps.ModuleFileSnapshot{Exists: true, Content: "old"},
		},
		Backup:       &deps.DependencyBackupInfo{Name: "backup.json", Path: "/tmp/backup.json"},
		Dependencies: dependencies,
	})
	got := updated.(Model)

	if got.Deps.Cycle.Phase() != deps.PhaseConfirmChecks {
		t.Fatalf("cycle phase = %s, want confirm-checks", got.Deps.Cycle.Phase())
	}
	if got.Deps.Cycle.Snapshot() == nil {
		t.Fatal("expected cycle snapshot")
	}
	if got.Deps.Dialog.Kind != DialogChecks || !got.Deps.Dialog.ChoiceYes {
		t.Fatalf("dialog = %+v, want checks default Yes", got.Deps.Dialog)
	}
	if !reflect.DeepEqual(got.Deps.Dependencies, dependencies) {
		t.Fatalf("dependencies = %+v, want %+v", got.Deps.Dependencies, dependencies)
	}
}

func TestChecksPassedCompletesCycle(t *testing.T) {
	m := modelAtConfirmChecks(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmChecksEvent{Yes: true})
	m.resetDialog()

	updated, _ := m.Update(deps.ChecksDoneEvent{
		Result: deps.DependencyCheckResult{OK: true},
	})
	got := updated.(Model)

	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if got.Deps.Dialog.Active() {
		t.Fatal("expected dialog closed")
	}
	if got.Status.Kind() != "success" {
		t.Fatalf("status kind = %q, want success", got.Status.Kind())
	}
}

func TestChecksFailedOpensRollbackDialog(t *testing.T) {
	m := modelAtConfirmChecks(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmChecksEvent{Yes: true})

	updated, _ := m.Update(deps.ChecksDoneEvent{
		Result: deps.DependencyCheckResult{
			Command: "go test ./...",
			Output:  "FAIL",
		},
	})
	got := updated.(Model)

	if got.Deps.Dialog.Kind != DialogRollback || !got.Deps.Dialog.ChoiceYes {
		t.Fatalf("dialog = %+v, want rollback default Yes", got.Deps.Dialog)
	}
	if got.Deps.Dialog.Inconclusive {
		t.Fatal("failed command should not be marked inconclusive")
	}
	if got.Deps.Dialog.CheckResult == nil || got.Deps.Dialog.CheckResult.Command != "go test ./..." {
		t.Fatalf("dialog check result = %+v", got.Deps.Dialog.CheckResult)
	}
	if got.Deps.Cycle.CheckResult() == nil || got.Deps.Cycle.CheckResult().Command != "go test ./..." {
		t.Fatalf("check result = %+v", got.Deps.Cycle.CheckResult())
	}
}

func TestChecksInconclusiveOpensDistinctRollbackDialog(t *testing.T) {
	m := modelAtConfirmChecks(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmChecksEvent{Yes: true})

	updated, _ := m.Update(deps.ChecksDoneEvent{Err: errors.New("could not start go test")})
	got := updated.(Model)

	if got.Deps.Dialog.Kind != DialogRollback || !got.Deps.Dialog.Inconclusive {
		t.Fatalf("dialog = %+v, want inconclusive rollback", got.Deps.Dialog)
	}
	if !strings.Contains(got.Status.Text(), "could not start go test") {
		t.Fatalf("status = %q", got.Status.Text())
	}
}

func TestRollbackResultUpdatesState(t *testing.T) {
	m := modelAtConfirmRollback(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmRollbackEvent{Yes: true})
	dependencies := []deps.ModuleDependency{{
		Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0",
	}}

	updated, _ := m.Update(deps.RollbackDoneEvent{Dependencies: dependencies})
	got := updated.(Model)

	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if !reflect.DeepEqual(got.Deps.Dependencies, dependencies) {
		t.Fatalf("dependencies = %+v, want %+v", got.Deps.Dependencies, dependencies)
	}
	if got.Status.Kind() != "success" {
		t.Fatalf("status kind = %q, want success", got.Status.Kind())
	}
}

func TestSuccessfulCompensationReportsRestoredUpdateFailure(t *testing.T) {
	m := modelAtConfirmApply(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmApplyEvent{Yes: true})
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ApplyUpdatesDoneEvent{
		Snapshot: &deps.DependencySnapshot{
			ModFile: deps.ModuleFileSnapshot{Exists: true, Content: "old"},
		},
		Backup: &deps.DependencyBackupInfo{Name: "backup.json", Path: "/tmp/backup.json"},
		Err:    errors.New("go get failed"),
	})

	updated, _ := m.Update(deps.CompensateDoneEvent{
		Dependencies: []deps.ModuleDependency{{Path: "github.com/example/lib", Version: "v1.0.0"}},
	})
	got := updated.(Model)

	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if !strings.Contains(got.Status.Text(), "go get failed") || !strings.Contains(got.Status.Text(), "reverted") {
		t.Fatalf("status = %q", got.Status.Text())
	}
}

func TestRecoveryRequiredShowsBackupLocation(t *testing.T) {
	m := modelAtConfirmRollback(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmRollbackEvent{Yes: true})

	updated, _ := m.Update(deps.RollbackDoneEvent{Err: errors.New("disk full")})
	got := updated.(Model)

	if !strings.Contains(got.Status.Text(), "backup.json") ||
		!strings.Contains(got.Status.Text(), "/tmp/backup.json") {
		t.Fatalf("status = %q, want backup name and path", got.Status.Text())
	}
}

func TestCycleExecutionErrorResetsState(t *testing.T) {
	m := modelAtConfirmApply(t)
	m.Deps.Cycle = mustCycleEvent(t, m.Deps.Cycle, deps.ConfirmApplyEvent{Yes: true})

	updated, _ := m.Update(dependencyExecutionErrMsg{Err: errors.New("invalid executor intent")})
	got := updated.(Model)

	if got.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle", got.Deps.Cycle.Phase())
	}
	if got.Status.Kind() != "error" {
		t.Fatalf("status kind = %q, want error", got.Status.Kind())
	}
}

// newVersionCacheTestModel builds a Model with a multi-version catalog
// (installed, uninstalled, active) so that Download/Switch/Delete
// handlers have a non-trivial starting state to mutate. The list and
// table are populated via replaceVersions (through seedVersions) so the
// model starts in a consistent state.
func newVersionCacheTestModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	seedVersions(t, &m, []utils.GoVersion{
		{Version: "1.24.4", Filename: "go1.24.4.darwin-arm64.tar.gz", Installed: true, Active: true, Path: "/p/1.24.4"},
		{Version: "1.25.0", Filename: "go1.25.0.darwin-arm64.tar.gz", Installed: false},
		{Version: "1.26.0", Filename: "go1.26.0.darwin-arm64.tar.gz", Installed: true, Active: false, Path: "/p/1.26.0"},
	})
	return m
}

// TestVersionHandlersKeepCachesConsistent dispatches each of the four
// version-mutating msgs through Update and asserts the postcondition
// that the Available Versions list and the Installed Versions table
// remain exact projections of the catalog afterwards.
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
