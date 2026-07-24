package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

// This file is the Bubbletea adapter for the standalone (non-cycle)
// dependency commands: manual refresh, lazy load, backups listing, and
// restore. It is the only place in the model that knows about tea.Cmd
// for these operations; the underlying work lives in internal/deps,
// which is tea-free. The update workflow (check -> apply -> checks ->
// rollback) is driven by deps.UpdateCycle through cycle_adapter.go and
// has no entry here.

// DependenciesMsg carries the list of module dependencies. It is the
// result of both ListModuleDependenciesCmd (lazy load) and
// CheckModuleDependencyUpdatesCmd (manual refresh).
type DependenciesMsg []deps.ModuleDependency

// DependencyBackupsMsg carries the saved dependency backups for the
// current module.
type DependencyBackupsMsg []deps.DependencyBackupInfo

// DependenciesRestoredMsg is sent after restoring a dependency backup.
type DependenciesRestoredMsg deps.DependencyRestoreResult

// DependencyErrMsg carries a dependency-related error without affecting
// the main error state. It is intentionally a plain struct (not an
// error) so that it does not satisfy the error interface and therefore
// does not collide with ErrMsg in type switches.
type DependencyErrMsg struct {
	Err error
}

// ListModuleDependenciesCmd lists current module dependencies without
// checking for updates online. Used for the lazy load on first visit
// to the Deps tab.
func ListModuleDependenciesCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependenciesMsg, error) {
		dependencies, err := deps.ListModuleDependencies(moduleDir)
		return DependenciesMsg(dependencies), err
	})
}

// CheckModuleDependencyUpdatesCmd lists module dependencies and checks
// for available updates online. Used for the manual refresh action.
func CheckModuleDependencyUpdatesCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependenciesMsg, error) {
		dependencies, err := deps.CheckModuleDependencyUpdates(moduleDir)
		return DependenciesMsg(dependencies), err
	})
}

// ListDependencyBackupsCmd lists saved dependency backups for the
// current module, newest first.
func ListDependencyBackupsCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependencyBackupsMsg, error) {
		backups, err := deps.ListDependencyBackups(moduleDir)
		return DependencyBackupsMsg(backups), err
	})
}

// RestoreDependencyBackupCmd restores a saved dependency backup by
// filename, saving the current files first as a pre-restore backup.
func RestoreDependencyBackupCmd(moduleDir, backupName string, backupLimit int) tea.Cmd {
	return dependencyCmd(func() (DependenciesRestoredMsg, error) {
		result, err := deps.RestoreDependencyBackup(moduleDir, backupName, backupLimit)
		return DependenciesRestoredMsg(result), err
	})
}

// dependencyCmd adapts a synchronous internal/deps call into a tea.Cmd.
// The returned tea.Msg is the typed result on success, or
// DependencyErrMsg wrapping the error on failure.
func dependencyCmd[T any](run func() (T, error)) tea.Cmd {
	return func() tea.Msg {
		result, err := run()
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		return result
	}
}
