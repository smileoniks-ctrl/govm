package model

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestDepsOperationKeysIgnoreInputWhileOperationInProgress(t *testing.T) {
	operations := []struct {
		name string
		set  func(*DepsState)
	}{
		{
			name: "checking",
			set: func(deps *DepsState) {
				deps.Checking = true
			},
		},
		{
			name: "updating",
			set: func(deps *DepsState) {
				deps.Updating = true
			},
		},
		{
			name: "running checks",
			set: func(deps *DepsState) {
				deps.RunningChecks = true
			},
		},
		{
			name: "rolling back",
			set: func(deps *DepsState) {
				deps.RollingBack = true
			},
		},
		{
			name: "loading backups",
			set: func(deps *DepsState) {
				deps.LoadingBackups = true
			},
		},
		{
			name: "restoring backup",
			set: func(deps *DepsState) {
				deps.RestoringBackup = true
			},
		},
	}
	keys := []string{"u", "r", "b"}

	for _, operation := range operations {
		for _, key := range keys {
			t.Run(operation.name+"/"+key, func(t *testing.T) {
				m := newTestModel(t)
				m.CurrentTab = DepsTab
				m.Deps.Loaded = true
				m.Message = "unchanged"
				m.MessageType = "info"
				operation.set(&m.Deps)
				wantState := dependencyOperationState(m.Deps)

				updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0])})
				got := updated.(Model)

				if cmd != nil {
					t.Fatal("expected no command while a dependency operation is in progress")
				}
				if got.Message != m.Message || got.MessageType != m.MessageType {
					t.Fatalf("message = (%q, %q), want (%q, %q)", got.Message, got.MessageType, m.Message, m.MessageType)
				}
				if !reflect.DeepEqual(dependencyOperationState(got.Deps), wantState) {
					t.Fatal("expected dependency state to remain unchanged")
				}
			})
		}
	}
}

type depsOperationState struct {
	moduleDir       string
	dependencies    []utils.ModuleDependency
	loaded          bool
	checking        bool
	updating        bool
	runningChecks   bool
	rollingBack     bool
	loadingBackups  bool
	restoringBackup bool
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
		checking:        deps.Checking,
		updating:        deps.Updating,
		runningChecks:   deps.RunningChecks,
		rollingBack:     deps.RollingBack,
		loadingBackups:  deps.LoadingBackups,
		restoringBackup: deps.RestoringBackup,
		snapshot:        deps.Snapshot,
		backups:         deps.Backups,
		lastCheckResult: deps.LastCheckResult,
		dialog:          deps.Dialog,
	}
}

func TestDependencyErrMsgClearsEveryOperationFlag(t *testing.T) {
	operations := []struct {
		name  string
		set   func(*DepsState)
		isSet func(DepsState) bool
	}{
		{
			name: "checking",
			set: func(deps *DepsState) {
				deps.Checking = true
			},
			isSet: func(deps DepsState) bool {
				return deps.Checking
			},
		},
		{
			name: "updating",
			set: func(deps *DepsState) {
				deps.Updating = true
			},
			isSet: func(deps DepsState) bool {
				return deps.Updating
			},
		},
		{
			name: "running checks",
			set: func(deps *DepsState) {
				deps.RunningChecks = true
			},
			isSet: func(deps DepsState) bool {
				return deps.RunningChecks
			},
		},
		{
			name: "rolling back",
			set: func(deps *DepsState) {
				deps.RollingBack = true
			},
			isSet: func(deps DepsState) bool {
				return deps.RollingBack
			},
		},
		{
			name: "loading backups",
			set: func(deps *DepsState) {
				deps.LoadingBackups = true
			},
			isSet: func(deps DepsState) bool {
				return deps.LoadingBackups
			},
		},
		{
			name: "restoring backup",
			set: func(deps *DepsState) {
				deps.RestoringBackup = true
			},
			isSet: func(deps DepsState) bool {
				return deps.RestoringBackup
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			m := newTestModel(t)
			operation.set(&m.Deps)

			updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("operation failed")})
			got := updated.(Model)

			if operation.isSet(got.Deps) {
				t.Fatal("expected operation flag to be cleared")
			}
		})
	}

	t.Run("all operation flags", func(t *testing.T) {
		m := newTestModel(t)
		m.Deps.Checking = true
		m.Deps.Updating = true
		m.Deps.RunningChecks = true
		m.Deps.RollingBack = true
		m.Deps.LoadingBackups = true
		m.Deps.RestoringBackup = true

		updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("operation failed")})
		got := updated.(Model)

		for _, operation := range operations {
			if operation.isSet(got.Deps) {
				t.Fatalf("expected %s flag to be cleared", operation.name)
			}
		}
	})
}
