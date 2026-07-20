package model

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// activeDepsPhases lists every DepsOperation value that represents an
// in-flight operation. OpIdle is intentionally excluded. Tests that
// need to exercise "any in-flight phase" range over this slice
// instead of poking individual booleans.
func activeDepsPhases() []DepsOperation {
	return []DepsOperation{
		OpChecking,
		OpUpdating,
		OpRunningChecks,
		OpRollingBack,
		OpLoadingBackups,
		OpRestoringBackup,
	}
}

func TestDepsOperationKeysIgnoreInputWhileOperationInProgress(t *testing.T) {
	keys := []string{"u", "r", "b"}

	for _, phase := range activeDepsPhases() {
		for _, key := range keys {
			t.Run(phaseName(phase)+"/"+key, func(t *testing.T) {
				m := newTestModel(t)
				m.CurrentTab = DepsTab
				m.Deps.Loaded = true
				m.Status.SetTab("unchanged", "info")
				m.Deps.Phase = phase
				wantState := dependencyOperationState(m.Deps)

				updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0])})
				got := updated.(Model)

				if cmd != nil {
					t.Fatal("expected no command while a dependency operation is in progress")
				}
				if got.Status.Text() != m.Status.Text() || got.Status.Kind() != m.Status.Kind() {
					t.Fatalf("message = (%q, %q), want (%q, %q)", got.Status.Text(), got.Status.Kind(), m.Status.Text(), m.Status.Kind())
				}
				if !reflect.DeepEqual(dependencyOperationState(got.Deps), wantState) {
					t.Fatal("expected dependency state to remain unchanged")
				}
			})
		}
	}
}

// phaseName returns a lowercase human-readable label for a DepsOperation
// value, used to name subtests.
func phaseName(p DepsOperation) string {
	switch p {
	case OpChecking:
		return "checking"
	case OpUpdating:
		return "updating"
	case OpRunningChecks:
		return "running checks"
	case OpRollingBack:
		return "rolling back"
	case OpLoadingBackups:
		return "loading backups"
	case OpRestoringBackup:
		return "restoring backup"
	default:
		return "idle"
	}
}

type depsOperationState struct {
	moduleDir       string
	dependencies    []utils.ModuleDependency
	loaded          bool
	phase           DepsOperation
	snapshot        *utils.DependencySnapshot
	backups         []utils.DependencyBackupInfo
	lastCheckResult *utils.DependencyCheckResultMsg
	dialog          ConfirmDialog
}

func dependencyOperationState(deps DepsState) depsOperationState {
	return depsOperationState{
		moduleDir:       deps.ModuleDir,
		dependencies:    deps.Dependencies,
		loaded:          deps.Loaded,
		phase:           deps.Phase,
		snapshot:        deps.Snapshot,
		backups:         deps.Backups,
		lastCheckResult: deps.LastCheckResult,
		dialog:          deps.Dialog,
	}
}

func TestDependencyErrMsgClearsEveryOperationFlag(t *testing.T) {
	for _, phase := range activeDepsPhases() {
		t.Run(phaseName(phase), func(t *testing.T) {
			m := newTestModel(t)
			m.Deps.Phase = phase

			updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("operation failed")})
			got := updated.(Model)

			if got.Deps.Phase != OpIdle {
				t.Fatalf("expected phase to reset to OpIdle, got %v", got.Deps.Phase)
			}
		})
	}
}

// TestDepsPhaseTransitions locks in the canonical phase-reset matrix:
// each completion message maps its corresponding in-flight phase back
// to OpIdle. This is the artefact that materialises the "tests check
// transitions, not field mutations" leverage claimed by the refactor.
func TestDepsPhaseTransitions(t *testing.T) {
	tests := []struct {
		name      string
		fromPhase DepsOperation
		msg       tea.Msg
	}{
		{
			name:      "checking + DependenciesMsg -> idle",
			fromPhase: OpChecking,
			msg:       utils.DependenciesMsg{},
		},
		{
			name:      "updating + DependenciesUpdatedMsg -> idle",
			fromPhase: OpUpdating,
			msg:       utils.DependenciesUpdatedMsg{},
		},
		{
			name:      "running checks + DependencyCheckResultMsg (ok) -> idle",
			fromPhase: OpRunningChecks,
			msg:       utils.DependencyCheckResultMsg{OK: true},
		},
		{
			name:      "running checks + DependencyCheckResultMsg (failed) -> idle",
			fromPhase: OpRunningChecks,
			msg:       utils.DependencyCheckResultMsg{OK: false, Command: "go build", Output: "fail"},
		},
		{
			name:      "rolling back + DependenciesRolledBackMsg -> idle",
			fromPhase: OpRollingBack,
			msg:       utils.DependenciesRolledBackMsg{},
		},
		{
			name:      "loading backups + DependencyBackupsMsg -> idle",
			fromPhase: OpLoadingBackups,
			msg:       utils.DependencyBackupsMsg{},
		},
		{
			name:      "restoring backup + DependenciesRestoredMsg -> idle",
			fromPhase: OpRestoringBackup,
			msg:       utils.DependenciesRestoredMsg{},
		},
		{
			name:      "any phase + DependencyErrMsg -> idle",
			fromPhase: OpUpdating,
			msg:       utils.DependencyErrMsg{Err: errors.New("boom")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.Deps.Phase = tt.fromPhase

			updated, _ := m.Update(tt.msg)
			got := updated.(Model)

			if got.Deps.Phase != OpIdle {
				t.Fatalf("expected phase = OpIdle after transition, got %v", got.Deps.Phase)
			}
		})
	}
}
