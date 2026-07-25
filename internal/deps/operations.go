package deps

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// dependencyOperation is the testable seam through which the
// dependency operations perform all side-effecting work (command
// execution, loading, backup save/load, file restore). Production
// code uses defaultDependencyOperation; tests inject fakes.
// Resolution happens outside this seam; operations receive a
// resolved moduleContext.
type dependencyOperation struct {
	restoreFiles func(moduleContext, *DependencySnapshot) error
	runCommand   func(moduleContext, ...string) ([]byte, error)
	load         func(moduleContext, bool) ([]ModuleDependency, error)
	saveBackup   func(moduleContext, *DependencySnapshot, string, int) (DependencyBackupInfo, error)
	loadBackup   func(moduleContext, string) (*DependencyBackup, error)
}

func defaultDependencyOperation() dependencyOperation {
	return dependencyOperation{
		restoreFiles: func(context moduleContext, snap *DependencySnapshot) error {
			return RestoreModuleFiles(context.Root, snap)
		},
		runCommand: func(context moduleContext, args ...string) ([]byte, error) {
			cmd := exec.Command("go", args...)
			cmd.Dir = context.Root
			return cmd.CombinedOutput()
		},
		load: func(context moduleContext, checkUpdates bool) ([]ModuleDependency, error) {
			return loadDependencies(context.Root, checkUpdates)
		},
		saveBackup: saveDependencyBackupResolvedWithRetention,
		loadBackup: loadDependencyBackupResolved,
	}
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

func (operation dependencyOperation) restore(context moduleContext, snap *DependencySnapshot) error {
	if operation.restoreFiles == nil {
		return RestoreModuleFiles(context.Root, snap)
	}
	return operation.restoreFiles(context, snap)
}

// ListModuleDependencies lists current module dependencies without
// checking for updates online. The provided moduleDir is treated as a
// starting directory; the actual module root is resolved via
// ResolveModuleRoot so the call works from any subfolder of a Go module.
func ListModuleDependencies(moduleDir string) ([]ModuleDependency, error) {
	return listModuleDependencies(moduleDir, false, defaultDependencyOperation())
}

// CheckModuleDependencyUpdates lists module dependencies and checks
// for available updates online.
func CheckModuleDependencyUpdates(moduleDir string) ([]ModuleDependency, error) {
	return listModuleDependencies(moduleDir, true, defaultDependencyOperation())
}

func listModuleDependencies(
	moduleDir string,
	checkUpdates bool,
	operation dependencyOperation,
) ([]ModuleDependency, error) {
	context, err := resolveModuleContext(moduleDir)
	if err != nil {
		return nil, err
	}
	return operation.load(context, checkUpdates)
}

// runModuleDependencyChecks runs `go test ./...` followed by
// `go vet ./...` in the module that contains moduleDir.
func runModuleDependencyChecks(
	moduleDir string,
	operation dependencyOperation,
) (DependencyCheckResult, error) {
	context, err := resolveModuleContext(moduleDir)
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

// applyModuleUpdates is the single orchestration for applying direct
// dependency updates, used by the default executor Operations. It
// snapshots the module files, saves a persistent pre-update backup,
// runs `go get` and `go mod tidy`, then refreshes the dependency list.
// On a `go get`/`tidy` failure the snapshot and backup are returned
// alongside the error so the caller (cycle/executor) can compensate.
func applyModuleUpdates(
	moduleDir string,
	entries []DependencyUpdateEntry,
	backupLimit int,
	operation dependencyOperation,
) (*DependencySnapshot, *DependencyBackupInfo, []ModuleDependency, error) {
	if len(entries) == 0 {
		return nil, nil, nil, fmt.Errorf("no direct dependency updates available")
	}

	context, err := resolveModuleContext(moduleDir)
	if err != nil {
		return nil, nil, nil, err
	}
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

// rollbackModuleDependencies restores go.mod and go.sum verbatim from
// snap (exact byte restore, no `go mod tidy`) and refreshes the
// dependency list offline. Used by the default executor Operations.
func rollbackModuleDependencies(
	moduleDir string,
	snap *DependencySnapshot,
	operation dependencyOperation,
) (DependencyRollbackResult, error) {
	context, err := resolveModuleContext(moduleDir)
	if err != nil {
		return DependencyRollbackResult{}, err
	}

	if err := operation.restore(context, snap); err != nil {
		return DependencyRollbackResult{}, err
	}

	dependencies, err := operation.load(context, false)
	if err != nil {
		return DependencyRollbackResult{}, err
	}

	return DependencyRollbackResult{
		Snapshot:     snap,
		Dependencies: dependencies,
	}, nil
}

// RestoreDependencyBackup restores go.mod and go.sum verbatim from a
// saved dependency backup (exact byte restore, no `go mod tidy`),
// saving the current files first as a pre-restore backup so the
// restore itself can be undone manually. The pre-restore backup is
// retained on disk.
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
	context, err := resolveModuleContext(moduleDir)
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
	if err := operation.restore(context, backup.Snapshot); err != nil {
		restoreErr := fmt.Errorf("restore backup module files: %w", err)
		if compensationErr := operation.restore(context, current); compensationErr != nil {
			return DependencyRestoreResult{}, errors.Join(
				restoreErr,
				fmt.Errorf("restore original module files after backup restore failure: %w", compensationErr),
			)
		}
		return DependencyRestoreResult{}, restoreErr
	}

	dependencies, err := operation.load(context, false)
	if err != nil {
		return DependencyRestoreResult{}, err
	}

	return DependencyRestoreResult{
		BackupName:    filepath.Base(backupName),
		BackupCreated: backup.CreatedAt,
		Dependencies:  dependencies,
	}, nil
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
