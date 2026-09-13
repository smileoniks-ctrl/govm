package model

import (
	"errors"

	"charm.land/bubbles/v2/table"
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
	// Marks holds the module paths the user marked for the next
	// update (see CONTEXT.md "Mark"). Keyed by path, never by row,
	// because the table hides indirect rows and is rebuilt often.
	Marks map[string]bool
	// RowPaths maps table row index to module path, refreshed by
	// updateDependencyTable, so the cursor can be resolved to a module.
	RowPaths []string
	// Executor returns the dependency executor bound to the given
	// backup limit. It is read before every operation so a mid-session
	// limit change in Settings is honoured. It is bound through
	// Model.BindDepsOperations: main.go binds a single deps.Executor
	// for ModuleDir; tests bind a fake to drive the Cycle and the
	// standalone operations without IO. Unbound, every operation
	// reports errDepsUnavailable through the ordinary error path.
	Executor func(backupLimit int) DepsExecutor
}

// DepsExecutor is the seam through which the Deps tab performs every
// side-effecting dependency operation. *deps.Executor satisfies it.
type DepsExecutor interface {
	Execute(intent deps.Intent) (deps.Event, error)
	List() ([]deps.ModuleDependency, error)
	CheckUpdates() ([]deps.ModuleDependency, error)
	Backups() ([]deps.DependencyBackupInfo, error)
	Restore(backupName string) (deps.DependencyRestoreResult, error)
}

// errDepsUnavailable is what every dependency operation returns while
// no DepsExecutor is bound. It surfaces as a status message instead of
// a nil dereference, so a Model built without BindDepsOperations
// degrades quietly rather than crashing the TUI.
var errDepsUnavailable = errors.New("dependency operations are unavailable")

// unavailableDepsExecutor is the DepsExecutor in force before
// BindDepsOperations: every method fails with errDepsUnavailable.
type unavailableDepsExecutor struct{}

func (unavailableDepsExecutor) Execute(deps.Intent) (deps.Event, error) {
	return nil, errDepsUnavailable
}
func (unavailableDepsExecutor) List() ([]deps.ModuleDependency, error) {
	return nil, errDepsUnavailable
}
func (unavailableDepsExecutor) CheckUpdates() ([]deps.ModuleDependency, error) {
	return nil, errDepsUnavailable
}
func (unavailableDepsExecutor) Backups() ([]deps.DependencyBackupInfo, error) {
	return nil, errDepsUnavailable
}
func (unavailableDepsExecutor) Restore(string) (deps.DependencyRestoreResult, error) {
	return deps.DependencyRestoreResult{}, errDepsUnavailable
}

// NewDepsState builds an empty DepsState with the given table model
// and module directory. The Cycle is a fresh idle value. No executor
// is bound yet: constructing the state never touches the go toolchain.
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

// Marked reports whether the module at path carries a Mark.
func (s DepsState) Marked(path string) bool { return s.Marks[path] }

// ToggleMark flips the Mark on path.
func (s *DepsState) ToggleMark(path string) {
	if s.Marks == nil {
		s.Marks = map[string]bool{}
	}
	if s.Marks[path] {
		delete(s.Marks, path)
		return
	}
	s.Marks[path] = true
}

// ClearMarks removes every Mark.
func (s *DepsState) ClearMarks() { s.Marks = nil }

// MarkedPaths returns the marked module paths in dependency-list
// order. Marks on modules that are no longer listed are ignored.
func (s DepsState) MarkedPaths() []string {
	if len(s.Marks) == 0 {
		return nil
	}
	paths := make([]string, 0, len(s.Marks))
	for _, d := range s.Dependencies {
		if s.Marks[d.Path] {
			paths = append(paths, d.Path)
		}
	}
	return paths
}

// ToggleMarkAll clears every mark when any exists, otherwise marks
// every listed row (the rows the current display mode shows). It
// reports whether marks were added. Whether a marked module actually
// moves is decided by the update plan.
func (s *DepsState) ToggleMarkAll() bool {
	if len(s.MarkedPaths()) > 0 || len(s.RowPaths) == 0 {
		s.ClearMarks()
		return false
	}
	s.Marks = make(map[string]bool, len(s.RowPaths))
	for _, p := range s.RowPaths {
		s.Marks[p] = true
	}
	return true
}

// cursorDependency resolves the table cursor to its module, mapping
// through RowPaths because hidden indirect rows shift indices.
func (s DepsState) cursorDependency() (deps.ModuleDependency, bool) {
	i := s.Table.Cursor()
	if i < 0 || i >= len(s.RowPaths) {
		return deps.ModuleDependency{}, false
	}
	path := s.RowPaths[i]
	for _, d := range s.Dependencies {
		if d.Path == path {
			return d, true
		}
	}
	return deps.ModuleDependency{}, false
}

// explicitModules returns the module set behind the explicit Update
// scope: the marked modules, or the module under the cursor when
// nothing is marked. nil when neither exists.
func (s DepsState) explicitModules() []string {
	if paths := s.MarkedPaths(); len(paths) > 0 {
		return paths
	}
	if d, found := s.cursorDependency(); found {
		return []string{d.Path}
	}
	return nil
}

// updateSelection builds the selection `u` starts the cycle with. The
// initial Update scope is "marked" when marks exist and "all direct
// dependencies" otherwise; the dialog lets the user switch between
// the two. ok is false when the list is empty.
func (s DepsState) updateSelection() (sel deps.UpdateSelection, ok bool) {
	if paths := s.MarkedPaths(); len(paths) > 0 {
		return deps.UpdateSelection{Modules: paths}, true
	}
	if len(s.RowPaths) == 0 {
		return deps.UpdateSelection{}, false
	}
	return deps.UpdateSelection{}, true
}
