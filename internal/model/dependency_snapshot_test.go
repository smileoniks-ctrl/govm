package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestDependencySnapshotLifecycle(t *testing.T) {
	snapshot := &utils.DependencySnapshot{}
	checkResult := &utils.DependencyCheckResultMsg{
		Command: "go test ./...",
	}

	updatedModel := func(t *testing.T) Model {
		t.Helper()

		m := newTestModel(t)
		updated, _ := m.Update(utils.DependenciesUpdatedMsg{
			Dependencies: []utils.ModuleDependency{{Path: "example.com/dependency"}},
			Snapshot:     snapshot,
		})
		return updated.(Model)
	}

	tests := []struct {
		name         string
		apply        func(t *testing.T, m Model) Model
		wantSnapshot bool
		wantCheck    bool
	}{
		{
			name: "update creates snapshot",
			apply: func(t *testing.T, m Model) Model {
				return updatedModel(t)
			},
			wantSnapshot: true,
		},
		{
			name: "failed checks preserve rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(utils.DependencyCheckResultMsg{Command: checkResult.Command})
				return updated.(Model)
			},
			wantSnapshot: true,
			wantCheck:    true,
		},
		{
			name: "passed checks clear rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true, Command: checkResult.Command})
				return updated.(Model)
			},
		},
		{
			name: "passed checks without snapshot clear rollback context",
			apply: func(t *testing.T, m Model) Model {
				m.Deps.LastCheckResult = checkResult
				updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true, Command: checkResult.Command})
				return updated.(Model)
			},
		},
		{
			name: "skip checks with no clears rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(tea.KeyPressMsg{Code: 'n'})
				return updated.(Model)
			},
		},
		{
			name: "skip checks with false choice clears rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				m.Deps.Dialog.ChoiceYes = false
				updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				return updated.(Model)
			},
		},
		{
			name: "keep update clears rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(utils.DependencyCheckResultMsg{Command: checkResult.Command})
				m = updated.(Model)
				updated, _ = m.Update(tea.KeyPressMsg{Code: 'n'})
				return updated.(Model)
			},
		},
		{
			name: "successful rollback clears rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(utils.DependencyCheckResultMsg{Command: checkResult.Command})
				m = updated.(Model)
				updated, _ = m.Update(utils.DependenciesRolledBackMsg{Snapshot: snapshot})
				return updated.(Model)
			},
		},
		{
			name: "successful restore clears rollback context",
			apply: func(t *testing.T, m Model) Model {
				m = updatedModel(t)
				updated, _ := m.Update(utils.DependencyCheckResultMsg{Command: checkResult.Command})
				m = updated.(Model)
				updated, _ = m.Update(utils.DependenciesRestoredMsg{BackupName: "backup"})
				return updated.(Model)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.apply(t, newTestModel(t))
			if (got.Deps.Snapshot != nil) != tt.wantSnapshot {
				t.Fatalf("Snapshot present = %t, want %t", got.Deps.Snapshot != nil, tt.wantSnapshot)
			}
			if (got.Deps.LastCheckResult != nil) != tt.wantCheck {
				t.Fatalf("LastCheckResult present = %t, want %t", got.Deps.LastCheckResult != nil, tt.wantCheck)
			}
		})
	}
}
