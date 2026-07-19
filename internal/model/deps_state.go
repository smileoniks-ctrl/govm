package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// DepsState groups everything that belongs to the "Deps" tab so the
// main Model struct does not have to expose 15+ fields at the top
// level. It owns the dependency table, the snapshot used for
// rollback, the in-flight flags, and the active confirmation dialog.
type DepsState struct {
	ModuleDir       string
	Table           table.Model
	Dependencies    []utils.ModuleDependency
	UpdateEntries   []utils.DependencyUpdateEntry
	Loaded          bool
	Checking        bool
	Updating        bool
	RunningChecks   bool
	RollingBack     bool
	LoadingBackups  bool
	RestoringBackup bool
	Snapshot        *utils.DependencySnapshot
	Backups         []utils.DependencyBackupInfo
	LastCheckResult *utils.DependencyCheckResultMsg
	Dialog          ConfirmDialog
}

// NewDepsState builds an empty DepsState with the given table model
// and module directory. It is intentionally cheap so that main.go
// can initialise it once at startup. The active dialog is the zero
// value of ConfirmDialog (Kind == DialogIdle, i.e. no dialog open);
// default Yes choices and cursor bounds are set when a dialog opens.
func NewDepsState(moduleDir string, tbl table.Model) DepsState {
	return DepsState{
		ModuleDir: moduleDir,
		Table:     tbl,
	}
}

func (s DepsState) operationInProgress() bool {
	return s.Checking ||
		s.Updating ||
		s.RunningChecks ||
		s.RollingBack ||
		s.LoadingBackups ||
		s.RestoringBackup
}

func (s *DepsState) clearRollbackContext() {
	s.Snapshot = nil
	s.LastCheckResult = nil
}
