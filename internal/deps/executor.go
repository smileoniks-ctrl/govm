package deps

import (
	"fmt"
)

const DefaultBackupLimit = 10

// Operations is the seam through which the Executor performs all
// side-effecting work. Tests substitute a mock implementation.
type Operations interface {
	CheckUpdates(moduleDir string) ([]ModuleDependency, error)
	ApplyUpdates(
		moduleDir string,
		entries []DependencyUpdateEntry,
		backupLimit int,
	) (
		snapshot *DependencySnapshot,
		backup *DependencyBackupInfo,
		dependencies []ModuleDependency,
		err error,
	)
	RestoreExact(
		moduleDir string,
		snapshot *DependencySnapshot,
	) ([]ModuleDependency, error)
	RunChecks(moduleDir string) (DependencyCheckResult, error)
}

// InvalidIntentError is returned by Execute when the intent is not
// operational (i.e. a confirmation intent or NoIntent).
type InvalidIntentError struct {
	Intent string
}

func (e InvalidIntentError) Error() string {
	return fmt.Sprintf("deps: executor cannot execute non-operational intent %q", e.Intent)
}

type Executor struct {
	ops         Operations
	backupLimit int
}

func NewExecutor(ops Operations, backupLimit int) *Executor {
	if ops == nil {
		ops = defaultOperations{}
	}
	if backupLimit < 1 {
		backupLimit = DefaultBackupLimit
	}
	return &Executor{ops: ops, backupLimit: backupLimit}
}

// Execute runs an operational intent and returns the corresponding
// event. Non-operational intents return InvalidIntentError.
func (e *Executor) Execute(intent Intent) (Event, error) {
	switch i := intent.(type) {
	case IntentCheckUpdates:
		return e.executeCheckUpdates(i), nil
	case IntentApplyUpdates:
		return e.executeApplyUpdates(i), nil
	case IntentCompensate:
		return e.executeCompensate(i), nil
	case IntentRunChecks:
		return e.executeRunChecks(i), nil
	case IntentRollback:
		return e.executeRollback(i), nil
	default:
		name := "<nil>"
		if intent != nil {
			name = intent.intentName()
		}
		return nil, InvalidIntentError{Intent: name}
	}
}

func (e *Executor) executeCheckUpdates(i IntentCheckUpdates) Event {
	deps, err := e.ops.CheckUpdates(i.ModuleDir)
	return CheckUpdatesDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeApplyUpdates(i IntentApplyUpdates) Event {
	snap, backup, deps, err := e.ops.ApplyUpdates(i.ModuleDir, i.Entries, e.backupLimit)
	return ApplyUpdatesDoneEvent{
		Snapshot:     snap,
		Backup:       backup,
		Dependencies: deps,
		Err:          err,
	}
}

func (e *Executor) executeCompensate(i IntentCompensate) Event {
	deps, err := e.ops.RestoreExact(i.ModuleDir, i.Snapshot)
	return CompensateDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeRunChecks(i IntentRunChecks) Event {
	result, err := e.ops.RunChecks(i.ModuleDir)
	return ChecksDoneEvent{Result: result, Err: err}
}

func (e *Executor) executeRollback(i IntentRollback) Event {
	deps, err := e.ops.RestoreExact(i.ModuleDir, i.Snapshot)
	return RollbackDoneEvent{Dependencies: deps, Err: err}
}

// Default production Operations. defaultOperations delegates to the
// same dependency-operation implementation used by the public
// top-level functions so there is a single source of orchestration.
type defaultOperations struct{}

func (defaultOperations) CheckUpdates(moduleDir string) ([]ModuleDependency, error) {
	return listModuleDependencies(moduleDir, true, defaultDependencyOperation())
}

func (defaultOperations) ApplyUpdates(
	moduleDir string,
	entries []DependencyUpdateEntry,
	backupLimit int,
) (*DependencySnapshot, *DependencyBackupInfo, []ModuleDependency, error) {
	return applyModuleUpdates(moduleDir, entries, backupLimit, defaultDependencyOperation())
}

// RestoreExact performs an exact byte restore of the snapshot files
// (no `go mod tidy`) and refreshes the dependency list offline. It is
// shared by rollback and compensation.
func (defaultOperations) RestoreExact(
	moduleDir string,
	snapshot *DependencySnapshot,
) ([]ModuleDependency, error) {
	result, err := rollbackModuleDependencies(moduleDir, snapshot, defaultDependencyOperation())
	if err != nil {
		return nil, err
	}
	return result.Dependencies, nil
}

func (defaultOperations) RunChecks(moduleDir string) (DependencyCheckResult, error) {
	return runModuleDependencyChecks(moduleDir, defaultDependencyOperation())
}
