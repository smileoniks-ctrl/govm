package deps

import "time"

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
