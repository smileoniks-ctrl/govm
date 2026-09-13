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

// listDependencyArgs builds the `go list` argv for loading the module
// graph. Online checks add -u (Latest) and -versions (candidate list
// for patch/minor targets, ADR 0003) in one call. -versions queries
// every module in `all`, including the main module, so -e keeps a
// lookup failure (a main module path without a dot, an unpublished
// path, an unreachable proxy) inside that module's Error field
// instead of aborting the whole listing.
func listDependencyArgs(checkUpdates bool) []string {
	args := []string{"list", "-mod=readonly", "-m", "-json"}
	if checkUpdates {
		args = append(args, "-e", "-u", "-versions")
	}
	return append(args, "all")
}

func loadDependencies(moduleDir string, checkUpdates bool) ([]ModuleDependency, error) {
	cmd := exec.Command("go", listDependencyArgs(checkUpdates)...)
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
			Versions []string
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
		if len(raw.Versions) > 0 {
			d.Versions = raw.Versions
		}

		deps = append(deps, d)
	}

	return deps, nil
}
