package deps

import (
	"fmt"
	"strings"
	"sync"
)

const DefaultBackupLimit = 10

// Operations is the seam through which the Executor performs all
// side-effecting work, for the update cycle and for the standalone
// list / backups / restore operations alike. Tests substitute a mock
// implementation. Operations receive a resolved moduleContext instead
// of a string moduleDir.
type Operations interface {
	// Load lists the module dependencies; checkUpdates adds the
	// online update and version lookup.
	Load(context moduleContext, checkUpdates bool) ([]ModuleDependency, error)
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
	ListBackups(context moduleContext) ([]DependencyBackupInfo, error)
	RestoreBackup(
		context moduleContext,
		backupName string,
		backupLimit int,
	) (DependencyRestoreResult, error)
}

// InvalidIntentError is returned by Execute when the intent is not
// operational (i.e. a confirmation intent or NoIntent).
type InvalidIntentError struct {
	Intent string
}

func (e InvalidIntentError) Error() string {
	return fmt.Sprintf("deps: executor cannot execute non-operational intent %q", e.Intent)
}

// Executor performs every side-effecting dependency operation for one
// Go module: the operational intents of the UpdateCycle plus the
// standalone list, check, backups and restore operations. It is the
// single entry point for the CLI and the TUI.
//
// The module context (root directory and module path) is resolved
// lazily, once, on the first operation: constructing an Executor never
// touches the go toolchain, so a TUI started outside a Go module still
// starts, and "not in a Go module" surfaces as the result of the first
// operation. The resolution is shared by every copy made with
// WithBackupLimit.
type Executor struct {
	module      *lazyModuleContext
	ops         Operations
	backupLimit int
}

// lazyModuleContext resolves a module context at most once and
// remembers the outcome, error included.
type lazyModuleContext struct {
	resolve func() (moduleContext, error)
	once    sync.Once
	context moduleContext
	err     error
}

func (l *lazyModuleContext) get() (moduleContext, error) {
	l.once.Do(func() { l.context, l.err = l.resolve() })
	return l.context, l.err
}

// NewExecutor creates an Executor for the module containing moduleDir
// with the DefaultBackupLimit. A nil ops selects the production
// Operations.
func NewExecutor(moduleDir string, ops Operations) *Executor {
	return newExecutor(&lazyModuleContext{
		resolve: func() (moduleContext, error) { return resolveModuleContext(moduleDir) },
	}, ops)
}

// NewExecutorWithContext creates an Executor with an already resolved
// moduleContext. Used by tests to inject a fake context.
func NewExecutorWithContext(context moduleContext, ops Operations) *Executor {
	return newExecutor(&lazyModuleContext{
		resolve: func() (moduleContext, error) { return context, nil },
	}, ops)
}

func newExecutor(module *lazyModuleContext, ops Operations) *Executor {
	if ops == nil {
		ops = newDefaultOperations()
	}
	return &Executor{module: module, ops: ops, backupLimit: DefaultBackupLimit}
}

// WithBackupLimit returns a copy of the Executor whose ApplyUpdates and
// Restore keep at most backupLimit backups per module. Values below 1
// select the DefaultBackupLimit. The copy shares the resolved module
// context, so it is cheap to make before every operation.
func (e *Executor) WithBackupLimit(backupLimit int) *Executor {
	if backupLimit < 1 {
		backupLimit = DefaultBackupLimit
	}
	return &Executor{module: e.module, ops: e.ops, backupLimit: backupLimit}
}

// List returns the module dependencies without going online.
func (e *Executor) List() ([]ModuleDependency, error) {
	context, err := e.module.get()
	if err != nil {
		return nil, err
	}
	return e.ops.Load(context, false)
}

// CheckUpdates returns the module dependencies with their available
// updates and known versions. It is the same operation the update
// cycle runs for IntentCheckUpdates.
func (e *Executor) CheckUpdates() ([]ModuleDependency, error) {
	context, err := e.module.get()
	if err != nil {
		return nil, err
	}
	return e.ops.Load(context, true)
}

// Backups lists the saved dependency backups of the module, newest
// first.
func (e *Executor) Backups() ([]DependencyBackupInfo, error) {
	context, err := e.module.get()
	if err != nil {
		return nil, err
	}
	return e.ops.ListBackups(context)
}

// Restore replaces go.mod and go.sum with the contents of the named
// backup (exact bytes, no `go mod tidy`), saving the current files
// first as a pre-restore backup so the restore itself can be undone.
func (e *Executor) Restore(backupName string) (DependencyRestoreResult, error) {
	context, err := e.module.get()
	if err != nil {
		return DependencyRestoreResult{}, err
	}
	return e.ops.RestoreBackup(context, backupName, e.backupLimit)
}

// Execute runs an operational intent and returns the corresponding
// event. Non-operational intents return InvalidIntentError; a module
// that cannot be resolved returns that error.
func (e *Executor) Execute(intent Intent) (Event, error) {
	if !IsOperational(intent) {
		name := "<nil>"
		if intent != nil {
			name = intent.intentName()
		}
		return nil, InvalidIntentError{Intent: name}
	}
	context, err := e.module.get()
	if err != nil {
		return nil, err
	}
	switch i := intent.(type) {
	case IntentCheckUpdates:
		return e.executeCheckUpdates(context, i), nil
	case IntentApplyUpdates:
		return e.executeApplyUpdates(context, i), nil
	case IntentCompensate:
		return e.executeCompensate(context, i), nil
	case IntentRunChecks:
		return e.executeRunChecks(context, i), nil
	case IntentRollback:
		return e.executeRollback(context, i), nil
	default:
		return nil, InvalidIntentError{Intent: intent.intentName()}
	}
}

func (e *Executor) executeCheckUpdates(context moduleContext, _ IntentCheckUpdates) Event {
	deps, err := e.ops.Load(context, true)
	return CheckUpdatesDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeApplyUpdates(context moduleContext, i IntentApplyUpdates) Event {
	snap, backup, deps, err := e.ops.ApplyUpdates(context, i.Entries, e.backupLimit)
	return ApplyUpdatesDoneEvent{
		Snapshot:     snap,
		Backup:       backup,
		Dependencies: deps,
		Err:          err,
	}
}

func (e *Executor) executeCompensate(context moduleContext, i IntentCompensate) Event {
	deps, err := e.ops.RestoreExact(context, i.Snapshot)
	return CompensateDoneEvent{Dependencies: deps, Err: err}
}

func (e *Executor) executeRunChecks(context moduleContext, _ IntentRunChecks) Event {
	result, err := e.ops.RunChecks(context)
	return ChecksDoneEvent{Result: result, Err: err}
}

func (e *Executor) executeRollback(context moduleContext, i IntentRollback) Event {
	deps, err := e.ops.RestoreExact(context, i.Snapshot)
	return RollbackDoneEvent{Dependencies: deps, Err: err}
}

// defaultOperations is the production Operations. Every exec/fs call
// goes through its dependencyOperation seam, so the orchestration
// (backup, go get, go mod tidy, refresh) is testable without a go
// toolchain: tests build defaultOperations{operation: fake} directly.
type defaultOperations struct {
	operation dependencyOperation
}

func newDefaultOperations() defaultOperations {
	return defaultOperations{operation: defaultDependencyOperation()}
}

func (o defaultOperations) Load(context moduleContext, checkUpdates bool) ([]ModuleDependency, error) {
	return o.operation.load(context, checkUpdates)
}

func (o defaultOperations) ListBackups(context moduleContext) ([]DependencyBackupInfo, error) {
	return listDependencyBackupsResolved(context)
}

func (o defaultOperations) ApplyUpdates(
	context moduleContext,
	entries []DependencyUpdateEntry,
	backupLimit int,
) (*DependencySnapshot, *DependencyBackupInfo, []ModuleDependency, error) {
	if len(entries) == 0 {
		return nil, nil, nil, fmt.Errorf("no direct dependency updates available")
	}

	operation := o.operation
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
func (o defaultOperations) RestoreExact(
	context moduleContext,
	snapshot *DependencySnapshot,
) ([]ModuleDependency, error) {
	operation := o.operation
	if err := operation.restore(context, snapshot); err != nil {
		return nil, err
	}
	return operation.load(context, false)
}

func (o defaultOperations) RunChecks(context moduleContext) (DependencyCheckResult, error) {
	operation := o.operation
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
