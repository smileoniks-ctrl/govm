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

// DependencyUpdateResult describes a completed direct-dependency update.
type DependencyUpdateResult struct {
	Updated      int
	Dependencies []ModuleDependency
	Snapshot     *DependencySnapshot
}

// DependencyRollbackResult describes a completed dependency rollback.
type DependencyRollbackResult struct {
	Snapshot     *DependencySnapshot
	Dependencies []ModuleDependency
}

// DependencyRestoreResult describes a completed dependency backup restore.
type DependencyRestoreResult struct {
	BackupName    string
	BackupCreated time.Time
	Dependencies  []ModuleDependency
}

// DependencyCheckResult reports the result of the post-update checks.
type DependencyCheckResult struct {
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

// DirectDependencyUpdateEntries returns immutable update entries for
// direct dependencies that have an available update.
func DirectDependencyUpdateEntries(deps []ModuleDependency) []DependencyUpdateEntry {
	var entries []DependencyUpdateEntry
	for _, d := range deps {
		if d.Indirect || d.Error != "" || d.Latest == "" || d.Latest == d.Version {
			continue
		}
		entries = append(entries, DependencyUpdateEntry{
			Path:       d.Path,
			OldVersion: d.Version,
			NewVersion: d.Latest,
		})
	}
	return entries
}

// Pluralize returns singular when n is one, otherwise plural.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// UpdateModuleDependencies runs `go get` for each update entry, then
// `go mod tidy`, and finally re-checks
// available updates. It takes a snapshot of go.mod and go.sum before
// running go get so the caller can roll back on check failure.
func UpdateModuleDependencies(
	moduleDir string,
	entries []DependencyUpdateEntry,
	backupLimit int,
) (DependencyUpdateResult, error) {
	return updateModuleDependencies(moduleDir, entries, backupLimit, defaultDependencyOperation())
}

func updateModuleDependencies(
	moduleDir string,
	entries []DependencyUpdateEntry,
	backupLimit int,
	operation dependencyOperation,
) (DependencyUpdateResult, error) {
	if len(entries) == 0 {
		return DependencyUpdateResult{}, fmt.Errorf("no direct dependency updates available")
	}

	context, err := operation.resolve(moduleDir)
	if err != nil {
		return DependencyUpdateResult{}, err
	}
	snap, err := SnapshotModuleFiles(context.Root)
	if err != nil {
		return DependencyUpdateResult{}, err
	}
	snap.Updatable = entries
	if _, err := operation.save(context, snap, DependencyBackupKindPreUpdate, backupLimit); err != nil {
		return DependencyUpdateResult{}, err
	}

	args := []string{"get"}
	for _, entry := range entries {
		args = append(args, fmt.Sprintf("%s@%s", entry.Path, entry.NewVersion))
	}

	if out, err := operation.runCommand(context.Root, args...); err != nil {
		return DependencyUpdateResult{}, fmt.Errorf("go get failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	if out, err := operation.runCommand(context.Root, "mod", "tidy"); err != nil {
		return DependencyUpdateResult{}, fmt.Errorf("go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	dependencies, err := operation.load(context.Root, true)
	if err != nil {
		return DependencyUpdateResult{}, err
	}

	return DependencyUpdateResult{
		Updated:      len(entries),
		Dependencies: dependencies,
		Snapshot:     snap,
	}, nil
}

// ListModuleDependencies lists current module dependencies
// without checking for updates online. The provided moduleDir is
// treated as a starting directory; the actual module root is
// resolved via ResolveModuleRoot so the call works from any
// subfolder of a Go module.
func ListModuleDependencies(moduleDir string) ([]ModuleDependency, error) {
	return listModuleDependencies(moduleDir, false, defaultDependencyOperation())
}

// CheckModuleDependencyUpdates lists module dependencies
// and checks for available updates online. The provided moduleDir is
// treated as a starting directory; the actual module root is
// resolved via ResolveModuleRoot so the call works from any
// subfolder of a Go module.
func CheckModuleDependencyUpdates(moduleDir string) ([]ModuleDependency, error) {
	return listModuleDependencies(moduleDir, true, defaultDependencyOperation())
}

func listModuleDependencies(
	moduleDir string,
	checkUpdates bool,
	operation dependencyOperation,
) ([]ModuleDependency, error) {
	root, err := operation.resolveRoot(moduleDir)
	if err != nil {
		return nil, err
	}
	return operation.load(root, checkUpdates)
}

// RunModuleDependencyChecks runs `go test ./...` followed by
// `go vet ./...` in the module that contains moduleDir.
func RunModuleDependencyChecks(moduleDir string) (DependencyCheckResult, error) {
	return runModuleDependencyChecks(moduleDir, defaultDependencyOperation())
}

func runModuleDependencyChecks(
	moduleDir string,
	operation dependencyOperation,
) (DependencyCheckResult, error) {
	root, err := operation.resolveRoot(moduleDir)
	if err != nil {
		return DependencyCheckResult{}, err
	}

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
		out, err := operation.runCommand(root, check.args...)
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

type dependencyOperation struct {
	resolveContext func(string) (moduleContext, error)
	resolveRoot    func(string) (string, error)
	restoreFiles   func(string, *DependencySnapshot) error
	runCommand     func(string, ...string) ([]byte, error)
	load           func(string, bool) ([]ModuleDependency, error)
	saveBackup     func(moduleContext, *DependencySnapshot, string, int) (DependencyBackupInfo, error)
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
		saveBackup: saveDependencyBackupResolvedWithRetention,
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

func (operation dependencyOperation) save(context moduleContext, snap *DependencySnapshot, kind string, backupLimit int) (DependencyBackupInfo, error) {
	if operation.saveBackup == nil {
		return saveDependencyBackupResolvedWithRetention(context, snap, kind, backupLimit)
	}
	return operation.saveBackup(context, snap, kind, backupLimit)
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
func RollbackModuleDependencies(
	moduleDir string,
	snap *DependencySnapshot,
) (DependencyRollbackResult, error) {
	return rollbackModuleDependencies(moduleDir, snap, defaultDependencyOperation())
}

func rollbackModuleDependencies(
	moduleDir string,
	snap *DependencySnapshot,
	operation dependencyOperation,
) (DependencyRollbackResult, error) {
	root, err := operation.resolveRoot(moduleDir)
	if err != nil {
		return DependencyRollbackResult{}, err
	}

	if err := operation.restore(root, snap); err != nil {
		return DependencyRollbackResult{}, err
	}

	if out, err := operation.runCommand(root, "mod", "tidy"); err != nil {
		return DependencyRollbackResult{}, fmt.Errorf(
			"rollback go mod tidy failed: %s: %w",
			strings.TrimSpace(string(out)),
			err,
		)
	}

	dependencies, err := operation.load(root, false)
	if err != nil {
		return DependencyRollbackResult{}, err
	}

	return DependencyRollbackResult{
		Snapshot:     snap,
		Dependencies: dependencies,
	}, nil
}

// RestoreDependencyBackup restores go.mod and go.sum from a saved
// dependency backup, saving the current files first as a pre-restore
// backup so the restore itself can be undone manually.
func RestoreDependencyBackup(
	moduleDir string,
	backupName string,
	backupLimit int,
) (DependencyRestoreResult, error) {
	return restoreDependencyBackup(moduleDir, backupName, backupLimit, defaultDependencyOperation())
}

func restoreDependencyBackup(
	moduleDir string,
	backupName string,
	backupLimit int,
	operation dependencyOperation,
) (DependencyRestoreResult, error) {
	context, err := operation.resolve(moduleDir)
	if err != nil {
		return DependencyRestoreResult{}, err
	}
	backup, err := operation.loadBackupResolved(context, backupName)
	if err != nil {
		return DependencyRestoreResult{}, err
	}
	current, err := SnapshotModuleFiles(context.Root)
	if err != nil {
		return DependencyRestoreResult{}, err
	}
	if _, err := operation.save(context, current, DependencyBackupKindPreRestore, backupLimit); err != nil {
		return DependencyRestoreResult{}, err
	}
	if err := operation.restore(context.Root, backup.Snapshot); err != nil {
		restoreErr := fmt.Errorf("restore backup module files: %w", err)
		if compensationErr := operation.restore(context.Root, current); compensationErr != nil {
			return DependencyRestoreResult{}, errors.Join(
				restoreErr,
				fmt.Errorf("restore original module files after backup restore failure: %w", compensationErr),
			)
		}
		return DependencyRestoreResult{}, restoreErr
	}

	if out, err := operation.runCommand(context.Root, "mod", "tidy"); err != nil {
		tidyErr := fmt.Errorf("restore go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)
		if rollbackErr := operation.restore(context.Root, current); rollbackErr != nil {
			return DependencyRestoreResult{}, errors.Join(
				tidyErr,
				fmt.Errorf("restore original module files after tidy failure: %w", rollbackErr),
			)
		}
		return DependencyRestoreResult{}, tidyErr
	}

	dependencies, err := operation.load(context.Root, false)
	if err != nil {
		return DependencyRestoreResult{}, err
	}

	return DependencyRestoreResult{
		BackupName:    filepath.Base(backupName),
		BackupCreated: backup.CreatedAt,
		Dependencies:  dependencies,
	}, nil
}

const maxCheckOutputLines = 8

func trimOutput(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > maxCheckOutputLines {
		lines = append(lines[:maxCheckOutputLines], fmt.Sprintf("… (%d more lines)", len(lines)-maxCheckOutputLines))
	}
	return strings.Join(lines, "\n")
}

func loadDependencies(moduleDir string, checkUpdates bool) ([]ModuleDependency, error) {
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
			return nil, fmt.Errorf("go list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("go list failed: %w", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(output)))
	deps := []ModuleDependency{}

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
			return nil, fmt.Errorf("failed to parse go list output: %w", err)
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

	return deps, nil
}
