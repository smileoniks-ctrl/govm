package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ModuleDependency represents a single Go module dependency.
type ModuleDependency struct {
	Path       string
	Version    string
	Latest     string
	Indirect   bool
	Deprecated string
	Error      string
}

// DependenciesMsg carries the list of module dependencies.
type DependenciesMsg []ModuleDependency

// DependencyErrMsg carries a dependency-related error
// without affecting the main error state.
// Note: this is intentionally a plain struct (not an error) so that it
// does not satisfy the `error` interface and therefore does not collide
// with `utils.ErrMsg` in type switches.
type DependencyErrMsg struct {
	Err error
}

// DependenciesUpdatedMsg is sent after a successful update of direct
// dependencies, carrying the refreshed list, the number of modules
// that were upgraded, and a snapshot of the module files taken before
// the update so that the user can roll back if checks fail.
type DependenciesUpdatedMsg struct {
	Updated      int
	Dependencies []ModuleDependency
	Snapshot     *DependencySnapshot
}

// DependenciesRolledBackMsg is sent after a successful rollback of
// direct dependencies, carrying the refreshed list and the snapshot
// that was applied.
type DependenciesRolledBackMsg struct {
	Snapshot     *DependencySnapshot
	Dependencies []ModuleDependency
}

// DependencyBackupsMsg carries the saved dependency backups for the
// current module.
type DependencyBackupsMsg []DependencyBackupInfo

// DependenciesRestoredMsg is sent after restoring module files from
// a saved dependency backup.
type DependenciesRestoredMsg struct {
	BackupName    string
	BackupCreated time.Time
	Dependencies  []ModuleDependency
}

// DependencyCheckResultMsg reports the result of the post-update
// checks. The Output field is trimmed to a few lines so it can fit
// inside the rollback dialog.
type DependencyCheckResultMsg struct {
	OK      bool
	Command string
	Output  string
}

// ModuleFileSnapshot holds the pre-update contents of a single module
// file. Exists is false when the file was not present in the project
// at the time of the snapshot.
type ModuleFileSnapshot struct {
	Exists  bool
	Content string
}

// DependencyUpdateEntry records the old and new versions of a single
// direct dependency that is about to be updated.
type DependencyUpdateEntry struct {
	Path       string
	OldVersion string
	NewVersion string
}

// DependencySnapshot captures everything needed to roll back an
// update: the original module files plus the per-module version diff.
type DependencySnapshot struct {
	ModFile   ModuleFileSnapshot
	SumFile   ModuleFileSnapshot
	Updatable []DependencyUpdateEntry
}

// ResolveModuleRoot returns the absolute path to the Go module root
// containing startDir. It runs `go env GOMOD` with cmd.Dir = startDir
// so the search walks up the directory tree from startDir. The
// resolved module root is filepath.Dir of the go.mod path reported
// by Go. When startDir is not inside a Go module (the command
// fails, the output is empty, or it points at os.DevNull) a wrapped
// error is returned mentioning startDir.
func ResolveModuleRoot(startDir string) (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = startDir
	out, err := cmd.Output()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("not in a Go module (%s): %s", startDir, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("not in a Go module (%s): %w", startDir, err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", fmt.Errorf("not in a Go module (%s)", startDir)
	}
	return filepath.Dir(gomod), nil
}

// SnapshotModuleFiles reads go.mod and go.sum from moduleDir and
// returns a snapshot of their current contents. It does not run any
// external command. Returns an error if go.mod is missing, since
// rolling back requires at least the module declaration.
func SnapshotModuleFiles(moduleDir string) (*DependencySnapshot, error) {
	snap := &DependencySnapshot{}

	modPath := filepath.Join(moduleDir, "go.mod")
	modBytes, err := os.ReadFile(modPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot: go.mod not found in %s", moduleDir)
		}
		return nil, fmt.Errorf("snapshot go.mod: %w", err)
	}
	snap.ModFile = ModuleFileSnapshot{Exists: true, Content: string(modBytes)}

	sumBytes, err := os.ReadFile(filepath.Join(moduleDir, "go.sum"))
	switch {
	case err == nil:
		snap.SumFile = ModuleFileSnapshot{Exists: true, Content: string(sumBytes)}
	case os.IsNotExist(err):
		snap.SumFile = ModuleFileSnapshot{Exists: false}
	default:
		return nil, fmt.Errorf("snapshot go.sum: %w", err)
	}

	return snap, nil
}

// RestoreModuleFiles writes snap.ModFile and snap.SumFile back to
// disk. If snap.SumFile.Exists is false, any existing go.sum is
// removed. The module file content is restored verbatim, so the
// caller is responsible for any further `go mod tidy` step.
func RestoreModuleFiles(moduleDir string, snap *DependencySnapshot) error {
	if snap == nil {
		return fmt.Errorf("restore: nil snapshot")
	}

	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(snap.ModFile.Content), 0644); err != nil {
		return fmt.Errorf("restore go.mod: %w", err)
	}

	sumPath := filepath.Join(moduleDir, "go.sum")
	if snap.SumFile.Exists {
		if err := os.WriteFile(sumPath, []byte(snap.SumFile.Content), 0644); err != nil {
			return fmt.Errorf("restore go.sum: %w", err)
		}
	} else {
		if err := os.Remove(sumPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove go.sum: %w", err)
		}
	}

	return nil
}

// UpdatableDirectDependencies returns direct dependencies that have
// an available update.
func UpdatableDirectDependencies(deps []ModuleDependency) []ModuleDependency {
	var out []ModuleDependency
	for _, d := range deps {
		if d.Indirect {
			continue
		}
		if d.Error != "" {
			continue
		}
		if d.Latest == "" || d.Latest == d.Version {
			continue
		}
		out = append(out, d)
	}
	return out
}

// UpdateModuleDependencies runs `go get` for each updatable direct
// dependency in deps, then `go mod tidy`, and finally re-checks
// available updates. It takes a snapshot of go.mod and go.sum before
// running go get so the caller can roll back on check failure.
func UpdateModuleDependencies(moduleDir string, deps []ModuleDependency) tea.Cmd {
	return updateModuleDependencies(moduleDir, deps, defaultDependencyOperation())
}

func updateModuleDependencies(moduleDir string, deps []ModuleDependency, operation dependencyOperation) tea.Cmd {
	updatable := UpdatableDirectDependencies(deps)
	if len(updatable) == 0 {
		return func() tea.Msg {
			return DependencyErrMsg{Err: fmt.Errorf("no direct dependency updates available")}
		}
	}

	entries := make([]DependencyUpdateEntry, 0, len(updatable))
	for _, d := range updatable {
		entries = append(entries, DependencyUpdateEntry{
			Path:       d.Path,
			OldVersion: d.Version,
			NewVersion: d.Latest,
		})
	}

	return func() tea.Msg {
		context, err := operation.resolve(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		snap, err := SnapshotModuleFiles(context.Root)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		snap.Updatable = entries
		if _, err := operation.save(context, snap, DependencyBackupKindPreUpdate); err != nil {
			return DependencyErrMsg{Err: err}
		}

		args := []string{"get"}
		for _, d := range updatable {
			args = append(args, fmt.Sprintf("%s@%s", d.Path, d.Latest))
		}

		if out, err := operation.runCommand(context.Root, args...); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("go get failed: %s: %w", strings.TrimSpace(string(out)), err)}
		}

		if out, err := operation.runCommand(context.Root, "mod", "tidy"); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)}
		}

		// Refresh dependency list with available updates so the user
		// can see the new state in the table.
		fresh := operation.load(context.Root, true)
		if errMsg, ok := fresh.(DependencyErrMsg); ok {
			return errMsg
		}
		depsMsg, ok := fresh.(DependenciesMsg)
		if !ok {
			return DependencyErrMsg{Err: fmt.Errorf("unexpected refresh result: %T", fresh)}
		}

		return DependenciesUpdatedMsg{
			Updated:      len(updatable),
			Dependencies: []ModuleDependency(depsMsg),
			Snapshot:     snap,
		}
	}
}

// ListDependencyBackupsCmd lists saved dependency backups for the
// module containing moduleDir.
func ListDependencyBackupsCmd(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		backups, err := ListDependencyBackups(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		return DependencyBackupsMsg(backups)
	}
}

// ListModuleDependencies lists current module dependencies
// without checking for updates online. The provided moduleDir is
// treated as a starting directory; the actual module root is
// resolved via ResolveModuleRoot so the call works from any
// subfolder of a Go module.
func ListModuleDependencies(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		root, err := ResolveModuleRoot(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		return loadDependencies(root, false)
	}
}

// CheckModuleDependencyUpdates lists module dependencies
// and checks for available updates online. The provided moduleDir is
// treated as a starting directory; the actual module root is
// resolved via ResolveModuleRoot so the call works from any
// subfolder of a Go module.
func CheckModuleDependencyUpdates(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		root, err := ResolveModuleRoot(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		return loadDependencies(root, true)
	}
}

// RunModuleDependencyChecks runs `go test ./...` followed by
// `go vet ./...` in the module that contains moduleDir. On success
// it returns DependencyCheckResultMsg{OK: true}. On failure it
// returns DependencyCheckResultMsg{OK: false, Command, Output}
// where Output is the combined stdout/stderr of the failing
// command, trimmed to a reasonable number of lines. The provided
// moduleDir is treated as a starting directory; the actual module
// root is resolved via ResolveModuleRoot so the call works from
// any subfolder of a Go module.
func RunModuleDependencyChecks(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		root, err := ResolveModuleRoot(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}

		checks := []struct {
			args    []string
			command string
		}{
			{[]string{"test", "./..."}, "go test ./..."},
			{[]string{"vet", "./..."}, "go vet ./..."},
		}

		for _, c := range checks {
			cmd := exec.Command("go", c.args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				return DependencyCheckResultMsg{
					OK:      false,
					Command: c.command,
					Output:  trimOutput(string(out)),
				}
			}
		}

		return DependencyCheckResultMsg{OK: true}
	}
}

type dependencyOperation struct {
	resolveContext func(string) (moduleContext, error)
	resolveRoot    func(string) (string, error)
	restoreFiles   func(string, *DependencySnapshot) error
	runCommand     func(string, ...string) ([]byte, error)
	load           func(string, bool) tea.Msg
	saveBackup     func(moduleContext, *DependencySnapshot, string) (DependencyBackupInfo, error)
	loadBackup     func(moduleContext, string) (*DependencyBackup, error)
}

func defaultDependencyOperation() dependencyOperation {
	return dependencyOperation{
		resolveContext: resolveModuleContext,
		resolveRoot:    ResolveModuleRoot,
		restoreFiles:   RestoreModuleFiles,
		runCommand: func(moduleDir string, args ...string) ([]byte, error) {
			cmd := exec.Command("go", args...)
			cmd.Dir = moduleDir
			return cmd.CombinedOutput()
		},
		load:       loadDependencies,
		saveBackup: saveDependencyBackupResolved,
		loadBackup: loadDependencyBackupResolved,
	}
}

func (operation dependencyOperation) resolve(moduleDir string) (moduleContext, error) {
	if operation.resolveContext != nil {
		return operation.resolveContext(moduleDir)
	}
	root, err := operation.resolveRoot(moduleDir)
	if err != nil {
		return moduleContext{}, err
	}
	return resolveModuleContext(root)
}

func (operation dependencyOperation) save(context moduleContext, snap *DependencySnapshot, kind string) (DependencyBackupInfo, error) {
	if operation.saveBackup == nil {
		return saveDependencyBackupResolved(context, snap, kind)
	}
	return operation.saveBackup(context, snap, kind)
}

func (operation dependencyOperation) loadBackupResolved(context moduleContext, name string) (*DependencyBackup, error) {
	if operation.loadBackup == nil {
		return loadDependencyBackupResolved(context, name)
	}
	return operation.loadBackup(context, name)
}

func (operation dependencyOperation) restore(moduleDir string, snap *DependencySnapshot) error {
	if operation.restoreFiles == nil {
		return RestoreModuleFiles(moduleDir, snap)
	}
	return operation.restoreFiles(moduleDir, snap)
}

// RollbackModuleDependencies restores go.mod and go.sum from snap,
// runs `go mod tidy` so the module cache and the restored files stay
// consistent, and refreshes the dependency list. The provided
// moduleDir is treated as a starting directory; the actual module
// root is resolved via ResolveModuleRoot so the call works from any
// subfolder of a Go module.
func RollbackModuleDependencies(moduleDir string, snap *DependencySnapshot) tea.Cmd {
	return rollbackModuleDependencies(moduleDir, snap, defaultDependencyOperation())
}

func rollbackModuleDependencies(moduleDir string, snap *DependencySnapshot, operation dependencyOperation) tea.Cmd {
	return func() tea.Msg {
		root, err := operation.resolveRoot(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}

		if err := operation.restore(root, snap); err != nil {
			return DependencyErrMsg{Err: err}
		}

		if out, err := operation.runCommand(root, "mod", "tidy"); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("rollback go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)}
		}

		fresh := operation.load(root, false)
		if errMsg, ok := fresh.(DependencyErrMsg); ok {
			return errMsg
		}
		depsMsg, ok := fresh.(DependenciesMsg)
		if !ok {
			return DependencyErrMsg{Err: fmt.Errorf("unexpected refresh result: %T", fresh)}
		}

		return DependenciesRolledBackMsg{
			Snapshot:     snap,
			Dependencies: []ModuleDependency(depsMsg),
		}
	}
}

// RestoreDependencyBackup restores go.mod and go.sum from a saved
// dependency backup, saving the current files first as a pre-restore
// backup so the restore itself can be undone manually.
func RestoreDependencyBackup(moduleDir, backupName string) tea.Cmd {
	return restoreDependencyBackup(moduleDir, backupName, defaultDependencyOperation())
}

func restoreDependencyBackup(moduleDir, backupName string, operation dependencyOperation) tea.Cmd {
	return func() tea.Msg {
		context, err := operation.resolve(moduleDir)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		backup, err := operation.loadBackupResolved(context, backupName)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		current, err := SnapshotModuleFiles(context.Root)
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		if _, err := operation.save(context, current, DependencyBackupKindPreRestore); err != nil {
			return DependencyErrMsg{Err: err}
		}
		if err := operation.restore(context.Root, backup.Snapshot); err != nil {
			restoreErr := fmt.Errorf("restore backup module files: %w", err)
			if compensationErr := operation.restore(context.Root, current); compensationErr != nil {
				return DependencyErrMsg{Err: errors.Join(
					restoreErr,
					fmt.Errorf("restore original module files after backup restore failure: %w", compensationErr),
				)}
			}
			return DependencyErrMsg{Err: restoreErr}
		}

		if out, err := operation.runCommand(context.Root, "mod", "tidy"); err != nil {
			tidyErr := fmt.Errorf("restore go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)
			if rollbackErr := operation.restore(context.Root, current); rollbackErr != nil {
				return DependencyErrMsg{Err: errors.Join(
					tidyErr,
					fmt.Errorf("restore original module files after tidy failure: %w", rollbackErr),
				)}
			}
			return DependencyErrMsg{Err: tidyErr}
		}

		fresh := operation.load(context.Root, false)
		if errMsg, ok := fresh.(DependencyErrMsg); ok {
			return errMsg
		}
		depsMsg, ok := fresh.(DependenciesMsg)
		if !ok {
			return DependencyErrMsg{Err: fmt.Errorf("unexpected refresh result: %T", fresh)}
		}

		return DependenciesRestoredMsg{
			BackupName:    filepath.Base(backupName),
			BackupCreated: backup.CreatedAt,
			Dependencies:  []ModuleDependency(depsMsg),
		}
	}
}

const maxCheckOutputLines = 8

// MaxCheckOutputLinesForTest exposes the bound for tests.
func MaxCheckOutputLinesForTest() int { return maxCheckOutputLines }

func trimOutput(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > maxCheckOutputLines {
		lines = append(lines[:maxCheckOutputLines], fmt.Sprintf("… (%d more lines)", len(lines)-maxCheckOutputLines))
	}
	return strings.Join(lines, "\n")
}

func loadDependencies(moduleDir string, checkUpdates bool) tea.Msg {
	args := []string{"list", "-mod=readonly", "-m", "-json"}
	if checkUpdates {
		args = append(args, "-u")
	}
	args = append(args, "all")

	cmd := exec.Command("go", args...)
	cmd.Dir = moduleDir

	output, err := cmd.Output()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && len(exitErr.Stderr) > 0 {
			return DependencyErrMsg{Err: fmt.Errorf("go list failed: %s", string(exitErr.Stderr))}
		}
		return DependencyErrMsg{Err: fmt.Errorf("go list failed: %w", err)}
	}

	dec := json.NewDecoder(strings.NewReader(string(output)))
	var deps []ModuleDependency

	for dec.More() {
		var raw struct {
			Path       string
			Version    string
			Main       bool
			Indirect   bool
			Deprecated string
			Error      *struct {
				Err string
			}
			Update *struct {
				Version string
			}
		}
		if err := dec.Decode(&raw); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("failed to parse go list output: %w", err)}
		}

		// Skip the main module itself.
		if raw.Main {
			continue
		}

		d := ModuleDependency{
			Path:     raw.Path,
			Version:  raw.Version,
			Indirect: raw.Indirect,
		}

		if raw.Deprecated != "" {
			d.Deprecated = raw.Deprecated
		}

		if raw.Error != nil {
			d.Error = raw.Error.Err
		}

		if raw.Update != nil {
			d.Latest = raw.Update.Version
		}

		deps = append(deps, d)
	}

	return DependenciesMsg(deps)
}
