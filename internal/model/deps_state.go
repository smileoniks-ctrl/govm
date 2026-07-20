package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// DepsOperation is the phase of the in-flight dependency operation, if
// any. Mirrors the shape of DialogKind: OpIdle is the zero value so a
// freshly constructed DepsState is already idle.
type DepsOperation int

const (
	OpIdle DepsOperation = iota
	OpChecking
	OpUpdating
	OpRunningChecks
	OpRollingBack
	OpLoadingBackups
	OpRestoringBackup
)

// DepsState groups everything that belongs to the "Deps" tab so the
// main Model struct does not have to expose 15+ fields at the top
// level. It owns the dependency table, the snapshot used for rollback,
// the in-flight operation phase, and the active confirmation dialog.
//
// Invariant: Phase != OpIdle implies Dialog.Kind == DialogIdle. A
// confirmation dialog only opens after a phase transitions back to
// OpIdle (e.g. DependenciesUpdatedMsg sets Phase = OpIdle and opens
// DialogChecks). The two never overlap.
type DepsState struct {
	ModuleDir       string
	Table           table.Model
	Dependencies    []utils.ModuleDependency
	UpdateEntries   []utils.DependencyUpdateEntry
	Loaded          bool
	Phase           DepsOperation
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
	return s.Phase != OpIdle
}

// SpinnerText returns the noun phrase to render next to the spinner while
// a phase is in-flight, or "" if the caller should fall back to its own
// status text. Every active phase returns a phrase so composeStatus can
// uniformly prefix the spinner frame; no phase relies on an imperative
// Status.SetGlobal call to surface its progress text. This keeps the
// status scope consistent (a refresh no longer leaks a global-scope
// "Checking for dependency updates..." that survives tab switches).
func (s DepsState) SpinnerText() string {
	switch s.Phase {
	case OpChecking:
		return "Checking for dependency updates"
	case OpUpdating:
		return "Updating dependencies"
	case OpRollingBack:
		return "Rolling back dependencies"
	case OpRestoringBackup:
		return "Restoring dependency backup"
	case OpRunningChecks:
		return "Running checks"
	case OpLoadingBackups:
		return "Loading dependency backups"
	default:
		return ""
	}
}

// Reset clears the in-flight phase. It is the single replacement for the
// six "if X { X = false }" branches that used to live in the
// DependencyErrMsg handler. Reset does NOT drop the rollback context
// (Snapshot / LastCheckResult); use clearRollbackContext for that, which
// has different semantics ("operation finished successfully, drop the
// snapshot") and is called from a different set of sites.
func (s *DepsState) Reset() {
	s.Phase = OpIdle
}

func (s *DepsState) clearRollbackContext() {
	s.Snapshot = nil
	s.LastCheckResult = nil
}
