package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// DepsDialogState groups the Yes/No toggles for the three dependency
// dialogs so the main Model struct does not have to expose 6 boolean
// fields at the top level.
type DepsDialogState struct {
	ConfirmingUpdate   bool
	UpdateChoiceYes    bool
	ConfirmingChecks   bool
	CheckChoiceYes     bool
	ConfirmingRollback bool
	RollbackChoiceYes  bool
}

// DepsState groups everything that belongs to the "Deps" tab so the
// main Model struct does not have to expose 15+ fields at the top
// level. It owns the dependency table, the snapshot used for
// rollback, the in-flight flags, and the dialog state.
type DepsState struct {
	ModuleDir       string
	Table           table.Model
	Dependencies    []utils.ModuleDependency
	Loaded          bool
	Checking        bool
	Updating        bool
	RunningChecks   bool
	RollingBack     bool
	Snapshot        *utils.DependencySnapshot
	LastCheckResult *utils.DependencyCheckResultMsg
	Dialog          DepsDialogState
}

// NewDepsState builds an empty DepsState with the given table model
// and module directory. It is intentionally cheap so that main.go
// can initialise it once at startup.
func NewDepsState(moduleDir string, tbl table.Model) DepsState {
	return DepsState{
		ModuleDir: moduleDir,
		Table:     tbl,
		Dialog: DepsDialogState{
			UpdateChoiceYes:   true,
			CheckChoiceYes:    true,
			RollbackChoiceYes: true,
		},
	}
}
