package model

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

func activeStandalonePhases() []DepsOperation {
	return []DepsOperation{
		OpChecking,
		OpLoadingBackups,
		OpRestoringBackup,
	}
}

func TestDepsOperationKeysIgnoreInputWhileOperationInProgress(t *testing.T) {
	keys := []string{"u", "r", "b"}

	for _, phase := range activeStandalonePhases() {
		for _, key := range keys {
			t.Run(standalonePhaseName(phase)+"/"+key, func(t *testing.T) {
				m := newTestModel(t)
				m.CurrentTab = DepsTab
				m.Deps.Loaded = true
				m.Status.SetTab("unchanged", "info")
				m.Deps.Phase = phase

				updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0])})
				got := updated.(Model)

				if cmd != nil {
					t.Fatal("expected no command while a dependency operation is in progress")
				}
				if got.Status.Text() != m.Status.Text() || got.Status.Kind() != m.Status.Kind() {
					t.Fatalf("message = (%q, %q), want (%q, %q)", got.Status.Text(), got.Status.Kind(), m.Status.Text(), m.Status.Kind())
				}
				if got.Deps.Phase != phase {
					t.Fatalf("phase = %v, want %v", got.Deps.Phase, phase)
				}
			})
		}
	}

	for _, cycle := range activeUpdateCycles(t) {
		for _, key := range keys {
			t.Run(cycle.Phase().String()+"/"+key, func(t *testing.T) {
				m := newTestModel(t)
				m.CurrentTab = DepsTab
				m.Deps.Loaded = true
				m.Deps.Cycle = cycle

				updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0])})
				got := updated.(Model)

				if cmd != nil {
					t.Fatal("expected no command while an update cycle is active")
				}
				if got.Deps.Cycle.Phase() != cycle.Phase() {
					t.Fatalf("cycle phase = %s, want %s", got.Deps.Cycle.Phase(), cycle.Phase())
				}
			})
		}
	}
}

func TestDependencyErrMsgClearsStandaloneOperation(t *testing.T) {
	for _, phase := range activeStandalonePhases() {
		t.Run(standalonePhaseName(phase), func(t *testing.T) {
			m := newTestModel(t)
			m.Deps.Phase = phase

			updated, _ := m.Update(DependencyErrMsg{Err: errors.New("operation failed")})
			got := updated.(Model)

			if got.Deps.Phase != OpIdle {
				t.Fatalf("phase = %v, want OpIdle", got.Deps.Phase)
			}
		})
	}
}

func activeUpdateCycles(t *testing.T) []deps.UpdateCycle {
	t.Helper()
	snapshot := &deps.DependencySnapshot{
		ModFile: deps.ModuleFileSnapshot{Exists: true, Content: "module example.com/app\n"},
	}
	backup := &deps.DependencyBackupInfo{Name: "backup.json", Path: "/tmp/backup.json"}
	dependencies := []deps.ModuleDependency{{
		Path: "example.com/dependency", Version: "v1.0.0", Latest: "v1.1.0",
	}}

	started := mustCycleEvent(t, deps.NewUpdateCycle(), deps.StartEvent{ModuleDir: "/tmp/module"})
	confirmApply := mustCycleEvent(t, started, deps.CheckUpdatesDoneEvent{Dependencies: dependencies})
	applying := mustCycleEvent(t, confirmApply, deps.ConfirmApplyEvent{Yes: true})
	compensating := mustCycleEvent(t, applying, deps.ApplyUpdatesDoneEvent{
		Snapshot: snapshot,
		Backup:   backup,
		Err:      errors.New("apply failed"),
	})
	confirmChecks := mustCycleEvent(t, applying, deps.ApplyUpdatesDoneEvent{
		Snapshot:     snapshot,
		Backup:       backup,
		Dependencies: dependencies,
	})
	runningChecks := mustCycleEvent(t, confirmChecks, deps.ConfirmChecksEvent{Yes: true})
	confirmRollback := mustCycleEvent(t, runningChecks, deps.ChecksDoneEvent{
		Result: deps.DependencyCheckResult{Command: "go test ./..."},
	})
	rollingBack := mustCycleEvent(t, confirmRollback, deps.ConfirmRollbackEvent{Yes: true})

	return []deps.UpdateCycle{
		started,
		confirmApply,
		applying,
		compensating,
		confirmChecks,
		runningChecks,
		confirmRollback,
		rollingBack,
	}
}

func mustCycleEvent(t *testing.T, cycle deps.UpdateCycle, event deps.Event) deps.UpdateCycle {
	t.Helper()
	next, _, err := cycle.Handle(event)
	if err != nil {
		t.Fatalf("handle %T: %v", event, err)
	}
	return next
}

func standalonePhaseName(phase DepsOperation) string {
	switch phase {
	case OpChecking:
		return "checking"
	case OpLoadingBackups:
		return "loading-backups"
	case OpRestoringBackup:
		return "restoring-backup"
	default:
		return "idle"
	}
}
