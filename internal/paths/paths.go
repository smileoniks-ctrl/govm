// Package paths centralises every on-disk location that govm
// touches. All runtime files (versions, shim, active version,
// downloads) live under a single root directory inside the user's
// home, traditionally $HOME/.govm. User-visible settings have been
// migrated to $HOME/.govm/settings.json but older installs may still
// carry a copy under the platform user config directory; the
// resolver exposes both locations so callers can perform a one-shot
// migration on load.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	rootDirName       = ".govm"
	versionsDirName   = "versions"
	downloadsDirName  = "downloads"
	shimDirName       = "shim"
	depsBackupDirName = "deps_backup"
	activeVersionName = "active_version"
	installLockName   = "install.lock"
	settingsFileName  = "settings.json"
	legacyGovmDirName = "govm"
)

// Resolver computes govm filesystem locations. HomeDir and ConfigDir
// are exposed as fields so tests can inject deterministic paths
// without touching the real filesystem.
type Resolver struct {
	// HomeDir returns the user home directory used as the parent of
	// the govm root. Defaults to os.UserHomeDir.
	HomeDir func() (string, error)
	// ConfigDir returns the platform user-config directory used as
	// the parent of the legacy settings file. Defaults to
	// os.UserConfigDir.
	ConfigDir func() (string, error)
}

// New returns a Resolver backed by the standard library helpers.
func New() *Resolver {
	return &Resolver{
		HomeDir:   os.UserHomeDir,
		ConfigDir: os.UserConfigDir,
	}
}

// IsDirectChild reports whether candidate is an immediate child of root.
func IsDirectChild(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && filepath.Dir(rel) == "." && !filepath.IsAbs(rel)
}

// RootDir returns the govm root directory (e.g. ~/.govm).
// It does not check that the directory exists.
func (r *Resolver) RootDir() (string, error) {
	home, err := r.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rootDirName), nil
}

// VersionsDir returns the directory that holds installed Go
// versions, e.g. ~/.govm/versions.
func (r *Resolver) VersionsDir() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, versionsDirName), nil
}

// DownloadsDir returns the directory that holds downloaded Go
// archives, e.g. ~/.govm/downloads.
func (r *Resolver) DownloadsDir() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, downloadsDirName), nil
}

// InstallationLockFile returns the cross-process installation lock path.
func (r *Resolver) InstallationLockFile() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, installLockName), nil
}

// ShimDir returns the directory that holds the shim scripts, e.g.
// ~/.govm/shim. This is the directory the user is expected to put
// on their PATH.
func (r *Resolver) ShimDir() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, shimDirName), nil
}

// ActiveVersionFile returns the path of the file that records the
// currently active Go version, e.g. ~/.govm/active_version.
func (r *Resolver) ActiveVersionFile() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, activeVersionName), nil
}

// SettingsFile returns the canonical location of the settings file,
// e.g. ~/.govm/settings.json. New installs should always read and
// write this file; older installs may still hold a copy under the
// platform user-config directory (see LegacySettingsFile).
func (r *Resolver) SettingsFile() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, settingsFileName), nil
}

// DepsBackupDir returns the directory that holds dependency backup
// snapshots, e.g. ~/.govm/deps_backup.
func (r *Resolver) DepsBackupDir() (string, error) {
	root, err := r.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, depsBackupDirName), nil
}

// LegacySettingsFile returns the historical location of the
// settings file under the platform user-config directory, e.g.
// ~/Library/Application Support/govm/settings.json on macOS or
// ~/.config/govm/settings.json on Linux. The bool result reports
// whether a legacy path could be resolved at all; an empty string
// with a nil error means the host has no platform user-config
// directory (rare, e.g. some minimal Linux containers).
func (r *Resolver) LegacySettingsFile() (string, bool, error) {
	configDir, err := r.configDir()
	if err != nil {
		return "", false, err
	}
	if configDir == "" {
		return "", false, nil
	}
	return filepath.Join(configDir, legacyGovmDirName, settingsFileName), true, nil
}

// homeDir centralises the HomeDir lookup with a clear error message
// so callers do not have to wrap os.UserHomeDir errors themselves.
func (r *Resolver) homeDir() (string, error) {
	fn := r.HomeDir
	if fn == nil {
		fn = os.UserHomeDir
	}
	home, err := fn()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("resolve home directory: empty path")
	}
	return home, nil
}

func (r *Resolver) configDir() (string, error) {
	fn := r.ConfigDir
	if fn == nil {
		fn = os.UserConfigDir
	}
	dir, err := fn()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return dir, nil
}
