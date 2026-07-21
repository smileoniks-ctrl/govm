package utils

import tea "charm.land/bubbletea/v2"

// DependenciesMsg carries the list of module dependencies.
type DependenciesMsg []ModuleDependency

// DependencyErrMsg carries a dependency-related error
// without affecting the main error state.
// Note: this is intentionally a plain struct (not an error) so that it
// does not satisfy the error interface and therefore does not collide
// with ErrMsg in type switches.
type DependencyErrMsg struct {
	Err error
}

// DependenciesUpdatedMsg is sent after a successful dependency update.
type DependenciesUpdatedMsg DependencyUpdateResult

// DependenciesRolledBackMsg is sent after a successful dependency rollback.
type DependenciesRolledBackMsg DependencyRollbackResult

// DependencyBackupsMsg carries the saved dependency backups for the
// current module.
type DependencyBackupsMsg []DependencyBackupInfo

// DependenciesRestoredMsg is sent after restoring a dependency backup.
type DependenciesRestoredMsg DependencyRestoreResult

// DependencyCheckResultMsg reports the result of post-update checks.
type DependencyCheckResultMsg DependencyCheckResult

// ListModuleDependenciesCmd lists current module dependencies.
func ListModuleDependenciesCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependenciesMsg, error) {
		dependencies, err := ListModuleDependencies(moduleDir)
		return DependenciesMsg(dependencies), err
	})
}

// CheckModuleDependencyUpdatesCmd checks for dependency updates.
func CheckModuleDependencyUpdatesCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependenciesMsg, error) {
		dependencies, err := CheckModuleDependencyUpdates(moduleDir)
		return DependenciesMsg(dependencies), err
	})
}

// UpdateModuleDependenciesCmd updates direct dependencies.
func UpdateModuleDependenciesCmd(
	moduleDir string,
	entries []DependencyUpdateEntry,
	backupLimit int,
) tea.Cmd {
	return dependencyCmd(func() (DependenciesUpdatedMsg, error) {
		result, err := UpdateModuleDependencies(moduleDir, entries, backupLimit)
		return DependenciesUpdatedMsg(result), err
	})
}

// RunModuleDependencyChecksCmd runs dependency checks.
func RunModuleDependencyChecksCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependencyCheckResultMsg, error) {
		result, err := RunModuleDependencyChecks(moduleDir)
		return DependencyCheckResultMsg(result), err
	})
}

// RollbackModuleDependenciesCmd rolls back a dependency update.
func RollbackModuleDependenciesCmd(moduleDir string, snap *DependencySnapshot) tea.Cmd {
	return dependencyCmd(func() (DependenciesRolledBackMsg, error) {
		result, err := RollbackModuleDependencies(moduleDir, snap)
		return DependenciesRolledBackMsg(result), err
	})
}

// ListDependencyBackupsCmd lists saved dependency backups.
func ListDependencyBackupsCmd(moduleDir string) tea.Cmd {
	return dependencyCmd(func() (DependencyBackupsMsg, error) {
		backups, err := ListDependencyBackups(moduleDir)
		return DependencyBackupsMsg(backups), err
	})
}

// RestoreDependencyBackupCmd restores a saved dependency backup.
func RestoreDependencyBackupCmd(
	moduleDir string,
	backupName string,
	backupLimit int,
) tea.Cmd {
	return dependencyCmd(func() (DependenciesRestoredMsg, error) {
		result, err := RestoreDependencyBackup(moduleDir, backupName, backupLimit)
		return DependenciesRestoredMsg(result), err
	})
}

func dependencyCmd[T any](run func() (T, error)) tea.Cmd {
	return func() tea.Msg {
		result, err := run()
		if err != nil {
			return DependencyErrMsg{Err: err}
		}
		return result
	}
}
