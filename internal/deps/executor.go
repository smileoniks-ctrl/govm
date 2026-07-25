package deps

import (
	"fmt"
	"strings"
)

const DefaultBackupLimit = 10

// Operations is the seam through which the Executor performs all
// side-effecting work. Tests substitute a mock implementation.
// Operations receive a resolved moduleContext instead of a string
// moduleDir.
type Operations interface {
	CheckUpdates(context moduleContext) ([]ModuleDependency, error)
	ApplyUpdates(
		context moduleContext,
		entries []DependencyUpdateEntry,
		backupLimit int,
	) (
		snapshot *DependencySnapshot,
		backup *DependencyBackupInfo,
		dependencies []ModuleDependency,
		err error,
	)
	RestoreExact(
		context moduleContext,
		snapshot *DependencySnapshot,
	) ([]ModuleDependency, error)
	RunChecks(context moduleContext) (DependencyCheckResult, error)
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
	context     moduleContext
	ops         Operations
	backupLimit int
}

// NewExecutor creates an Executor for the module containing moduleDir.
// It resolves the module context (root directory and module path) once
// at construction. Resolution failure returns an error.
func NewExecutor(moduleDir string, ops Operations, backupLimit int) (*Executor, error) {
	context, err := resolveModuleContext(moduleDir)
	if err != nil {
		return nil, err
	}
	if ops == nil {
		ops = defaultOperations{}
	}
	if backupLimit < 1 {
		backupLimit = DefaultBackupLimit
	}
	return &Executor{
		context:     context,
		ops:         ops,
		backupLimit: backupLimit,
	}, nil
}

// NewExecutorWithContext creates an Executor with an already resolved
// moduleContext. Used by tests to inject a fake context.
func NewExecutorWithContext(context moduleContext, ops Operations, backupLimit int) *Executor {
	if ops == nil {
		ops = defaultOperations{}
	}
	if backupLimit < 1 {
		backupLimit = DefaultBackupLimit
	}
	return &Executor{
		context:     context,
		ops:         ops,
		backupLimit: backupLimit,
	}
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
	deps, err := e.ops.CheckUpdates(e.context)
	return CheckUpdatesDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeApplyUpdates(i IntentApplyUpdates) Event {
	snap, backup, deps, err := e.ops.ApplyUpdates(e.context, i.Entries, e.backupLimit)
	return ApplyUpdatesDoneEvent{
		Snapshot:     snap,
		Backup:       backup,
		Dependencies: deps,
		Err:          err,
	}
}

func (e *Executor) executeCompensate(i IntentCompensate) Event {
	deps, err := e.ops.RestoreExact(e.context, i.Snapshot)
	return CompensateDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeRunChecks(i IntentRunChecks) Event {
	result, err := e.ops.RunChecks(e.context)
	return ChecksDoneEvent{Result: result, Err: err}
}

func (e *Executor) executeRollback(i IntentRollback) Event {
	deps, err := e.ops.RestoreExact(e.context, i.Snapshot)
	return RollbackDoneEvent{Dependencies: deps, Err: err}
}

// Default production Operations. defaultOperations delegates to
// internal orchestration functions that accept moduleContext.
type defaultOperations struct{}

func (defaultOperations) CheckUpdates(context moduleContext) ([]ModuleDependency, error) {
	operation := defaultDependencyOperation()
	return operation.load(context, true)
}

func (defaultOperations) ApplyUpdates(
	context moduleContext,
	entries []DependencyUpdateEntry,
	backupLimit int,
) (*DependencySnapshot, *DependencyBackupInfo, []ModuleDependency, error) {
	if len(entries) == 0 {
		return nil, nil, nil, fmt.Errorf("no direct dependency updates available")
	}

	operation := defaultDependencyOperation()
	snap, err := SnapshotModuleFiles(context.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	snap.Updatable = entries
	backup, err := operation.save(context, snap, DependencyBackupKindPreUpdate, backupLimit)
	if err != nil {
		return nil, nil, nil, err
	}

	args := []string{"get"}
	for _, entry := range entries {
		args = append(args, fmt.Sprintf("%s@%s", entry.Path, entry.NewVersion))
	}

	if out, err := operation.runCommand(context, args...); err != nil {
		return snap, &backup, nil, fmt.Errorf("go get failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	if out, err := operation.runCommand(context, "mod", "tidy"); err != nil {
		return snap, &backup, nil, fmt.Errorf("go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	dependencies, err := operation.load(context, true)
	if err != nil {
		return snap, &backup, nil, err
	}

	return snap, &backup, dependencies, nil
}

// RestoreExact performs an exact byte restore of the snapshot files
// (no `go mod tidy`) and refreshes the dependency list offline. It is
// shared by rollback and compensation.
func (defaultOperations) RestoreExact(
	context moduleContext,
	snapshot *DependencySnapshot,
) ([]ModuleDependency, error) {
	operation := defaultDependencyOperation()
	if err := operation.restore(context, snapshot); err != nil {
		return nil, err
	}
	return operation.load(context, false)
}

func (defaultOperations) RunChecks(context moduleContext) (DependencyCheckResult, error) {
	operation := defaultDependencyOperation()
	checks := []struct {
		args    []string
		command string
	}{
		{
			args:    []string{"test", "./..."},
			command: "go test ./...",
		},
		{
			args:    []string{"vet", "./..."},
			command: "go vet ./...",
		},
	}

	for _, check := range checks {
		out, err := operation.runCommand(context, check.args...)
		if err != nil {
			return DependencyCheckResult{
				OK:      false,
				Command: check.command,
				Output:  trimOutput(string(out)),
			}, nil
		}
	}

	return DependencyCheckResult{OK: true}, nil
}

func trimOutput(out string) string {
	const maxCheckOutputLines = 8
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > maxCheckOutputLines {
		lines = append(lines[:maxCheckOutputLines], fmt.Sprintf("… (%d more lines)", len(lines)-maxCheckOutputLines))
	}
	return strings.Join(lines, "\n")
}
