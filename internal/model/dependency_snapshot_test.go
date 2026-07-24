package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

func TestCycleAdapterKeepsRollbackContextUntilDecision(t *testing.T) {
	snapshot := &deps.DependencySnapshot{
		ModFile: deps.ModuleFileSnapshot{Exists: true, Content: "module example.com/app\n"},
	}
	backup := &deps.DependencyBackupInfo{Name: "backup.json", Path: "/tmp/backup.json"}
	dependencies := []deps.ModuleDependency{{
		Path: "example.com/dependency", Version: "v1.0.0", Latest: "v1.1.0",
	}}

	m := newTestModel(t)
	cycle := mustCycleEvent(t, deps.NewUpdateCycle(), deps.StartEvent{ModuleDir: "/tmp/module"})
	cycle = mustCycleEvent(t, cycle, deps.CheckUpdatesDoneEvent{Dependencies: dependencies})
	cycle = mustCycleEvent(t, cycle, deps.ConfirmApplyEvent{Yes: true})
	m.Deps.Cycle = cycle

	updated, _ := m.Update(deps.ApplyUpdatesDoneEvent{
		Snapshot:     snapshot,
		Backup:       backup,
		Dependencies: dependencies,
	})
	m = updated.(Model)
	if m.Deps.Cycle.Snapshot() == nil {
		t.Fatal("cycle should retain the snapshot while checks are pending")
	}
	if m.Deps.Dialog.Kind != DialogChecks {
		t.Fatalf("dialog kind = %v, want DialogChecks", m.Deps.Dialog.Kind)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.Deps.Cycle.Phase() != deps.PhaseRunningChecks {
		t.Fatalf("cycle phase = %s, want running-checks", m.Deps.Cycle.Phase())
	}

	updated, _ = m.Update(deps.ChecksDoneEvent{
		Result: deps.DependencyCheckResult{Command: "go test ./...", Output: "FAIL"},
	})
	m = updated.(Model)
	if m.Deps.Cycle.Snapshot() == nil || m.Deps.Cycle.CheckResult() == nil {
		t.Fatal("cycle should retain rollback context while rollback is pending")
	}
	if m.Deps.Dialog.Kind != DialogRollback {
		t.Fatalf("dialog kind = %v, want DialogRollback", m.Deps.Dialog.Kind)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
	m = updated.(Model)
	if m.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle phase = %s, want idle after terminal rendering", m.Deps.Cycle.Phase())
	}
	if m.Deps.Cycle.Snapshot() != nil || m.Deps.Cycle.CheckResult() != nil {
		t.Fatal("terminal adapter state should not retain rollback context")
	}
}
