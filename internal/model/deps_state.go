package model

import (
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

// DepsOperation tracks standalone (non-cycle) dependency operations:
// manual refresh, lazy load, backups listing, and restore. The update
// workflow (check -> apply -> checks -> rollback) is driven by the
// deps.UpdateCycle and has no entry here.
type DepsOperation int

const (
	OpIdle DepsOperation = iota
	OpChecking
	OpLoadingBackups
	OpRestoringBackup
)

// DepsState groups everything that belongs to the "Deps" tab. The
// deps.UpdateCycle owns the update-workflow phase, pending decision,
// entries, snapshot, and check context. DepsState retains only
// presentation/standalone state: the dependency table projection, the
// standalone operation phase, saved backups for the restore flow, and
// the active confirmation dialog.
type DepsState struct {
	ModuleDir    string
	Table        table.Model
	Dependencies []deps.ModuleDependency
	Loaded       bool
	Phase        DepsOperation
	Backups      []deps.DependencyBackupInfo
	Dialog       ConfirmDialog
	Cycle        deps.UpdateCycle
	// ExecuteIntent is the injectable execution seam that maps an
	// operational deps.Intent to a tea.Cmd. nil in production (the
	// adapter builds a real deps.Executor per operation from the
	// current Settings backup limit); tests inject a fake to drive the
	// Cycle without IO.
	ExecuteIntent func(deps.Intent) tea.Cmd
}

// NewDepsState builds an empty DepsState with the given table model
// and module directory. The Cycle is a fresh idle value.
func NewDepsState(moduleDir string, tbl table.Model) DepsState {
	return DepsState{
		ModuleDir: moduleDir,
		Table:     tbl,
		Cycle:     deps.NewUpdateCycle(),
	}
}

// operationInProgress reports whether any dependency operation —
// standalone or update-cycle — is in flight.
func (s DepsState) operationInProgress() bool {
	if s.Phase != OpIdle {
		return true
	}
	p := s.Cycle.Phase()
	return p != deps.PhaseIdle && p != deps.PhaseTerminal
}

// SpinnerText returns the noun phrase to render next to the spinner
// while an operation is in-flight, or "" if the caller should fall
// back to its own status text.
func (s DepsState) SpinnerText() string {
	switch s.Phase {
	case OpChecking:
		return "Checking for dependency updates"
	case OpRestoringBackup:
		return "Restoring dependency backup"
	case OpLoadingBackups:
		return "Loading dependency backups"
	}
	switch s.Cycle.Phase() {
	case deps.PhaseChecking:
		return "Checking for dependency updates"
	case deps.PhaseApplying:
		return "Updating dependencies"
	case deps.PhaseCompensating:
		return "Reverting partial update"
	case deps.PhaseRunningChecks:
		return "Running checks"
	case deps.PhaseRollingBack:
		return "Rolling back dependencies"
	}
	return ""
}

// Reset clears the standalone operation phase. It does not touch the
// Cycle; cycle errors are handled by the cycle adapter.
func (s *DepsState) Reset() {
	s.Phase = OpIdle
}
