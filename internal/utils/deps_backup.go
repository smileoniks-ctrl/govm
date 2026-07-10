package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/paths"
	"golang.org/x/mod/modfile"
)

const (
	DependencyBackupKindPreUpdate  = "pre-update"
	DependencyBackupKindPreRestore = "pre-restore"
	dependencyBackupSchemaVersion  = 1
)

var dependencyBackupNow = time.Now

// DependencyBackup is the on-disk JSON format for a dependency snapshot.
type DependencyBackup struct {
	SchemaVersion int                 `json:"schema_version"`
	CreatedAt     time.Time           `json:"created_at"`
	ModulePath    string              `json:"module_path"`
	ModuleDir     string              `json:"module_dir"`
	Kind          string              `json:"kind"`
	Snapshot      *DependencySnapshot `json:"snapshot"`
}

// DependencyBackupInfo is a compact listing item for backup selection.
type DependencyBackupInfo struct {
	Name       string
	Path       string
	CreatedAt  time.Time
	ModulePath string
	Kind       string
	Updated    int
}

func SaveDependencyBackup(moduleDir string, snap *DependencySnapshot, kind string) (DependencyBackupInfo, error) {
	if snap == nil {
		return DependencyBackupInfo{}, fmt.Errorf("save dependency backup: nil snapshot")
	}
	modulePath, err := ReadModulePath(moduleDir)
	if err != nil {
		return DependencyBackupInfo{}, err
	}
	root, err := ResolveModuleRoot(moduleDir)
	if err != nil {
		return DependencyBackupInfo{}, err
	}

	createdAt := dependencyBackupNow().UTC()
	dir, err := dependencyBackupProjectDir(modulePath)
	if err != nil {
		return DependencyBackupInfo{}, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return DependencyBackupInfo{}, fmt.Errorf("create dependency backup dir: %w", err)
	}

	backup := DependencyBackup{
		SchemaVersion: dependencyBackupSchemaVersion,
		CreatedAt:     createdAt,
		ModulePath:    modulePath,
		ModuleDir:     root,
		Kind:          kind,
		Snapshot:      snap,
	}
	bytes, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return DependencyBackupInfo{}, fmt.Errorf("marshal dependency backup: %w", err)
	}

	name, path, err := writeDependencyBackupFile(dir, createdAt, bytes)
	if err != nil {
		return DependencyBackupInfo{}, fmt.Errorf("write dependency backup: %w", err)
	}

	return backupInfo(name, path, backup), nil
}

func ListDependencyBackups(moduleDir string) ([]DependencyBackupInfo, error) {
	modulePath, err := ReadModulePath(moduleDir)
	if err != nil {
		return nil, err
	}
	dir, err := dependencyBackupProjectDir(modulePath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []DependencyBackupInfo{}, nil
		}
		return nil, fmt.Errorf("read dependency backups: %w", err)
	}

	backups := make([]DependencyBackupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		backup, err := readDependencyBackupFile(path)
		if err != nil {
			continue
		}
		if backup.ModulePath != modulePath {
			continue
		}
		backups = append(backups, backupInfo(entry.Name(), path, backup))
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	return backups, nil
}

func LoadDependencyBackup(moduleDir, name string) (*DependencyBackup, error) {
	modulePath, err := ReadModulePath(moduleDir)
	if err != nil {
		return nil, err
	}
	dir, err := dependencyBackupProjectDir(modulePath)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(name)
	backup, err := readDependencyBackupFile(filepath.Join(dir, base))
	if err != nil {
		return nil, err
	}
	if backup.ModulePath != modulePath {
		return nil, fmt.Errorf("backup %q belongs to module %q, current module is %q", base, backup.ModulePath, modulePath)
	}
	return &backup, nil
}

func ReadModulePath(moduleDir string) (string, error) {
	root, err := ResolveModuleRoot(moduleDir)
	if err != nil {
		return "", err
	}
	bytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	modulePath := modfile.ModulePath(bytes)
	if modulePath == "" {
		return "", fmt.Errorf("read go.mod: module path not found")
	}
	return modulePath, nil
}

func dependencyBackupProjectDir(modulePath string) (string, error) {
	root, err := paths.New().DepsBackupDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, sanitizeModulePath(modulePath))
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", fmt.Errorf("resolve dependency backup dir: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("resolve dependency backup dir: module path escapes backup root")
	}
	return dir, nil
}

func writeDependencyBackupFile(dir string, createdAt time.Time, bytes []byte) (string, string, error) {
	base := createdAt.Format("2006-01-02_15-04-05")
	for i := 0; i < 1000; i++ {
		name := base + ".json"
		if i > 0 {
			name = fmt.Sprintf("%s-%03d.json", base, i)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", "", err
		}
		if _, err := file.Write(bytes); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", "", err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return "", "", err
		}
		return name, path, nil
	}
	return "", "", fmt.Errorf("too many dependency backup name collisions for %s", base)
}

func readDependencyBackupFile(path string) (DependencyBackup, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return DependencyBackup{}, fmt.Errorf("read dependency backup: %w", err)
	}
	var backup DependencyBackup
	if err := json.Unmarshal(bytes, &backup); err != nil {
		return DependencyBackup{}, fmt.Errorf("parse dependency backup: %w", err)
	}
	if backup.SchemaVersion != dependencyBackupSchemaVersion {
		return DependencyBackup{}, fmt.Errorf("unsupported dependency backup schema version %d", backup.SchemaVersion)
	}
	if backup.Snapshot == nil {
		return DependencyBackup{}, fmt.Errorf("dependency backup %q has no snapshot", filepath.Base(path))
	}
	return backup, nil
}

func backupInfo(name, path string, backup DependencyBackup) DependencyBackupInfo {
	updated := 0
	if backup.Snapshot != nil {
		updated = len(backup.Snapshot.Updatable)
	}
	return DependencyBackupInfo{
		Name:       name,
		Path:       path,
		CreatedAt:  backup.CreatedAt,
		ModulePath: backup.ModulePath,
		Kind:       backup.Kind,
		Updated:    updated,
	}
}

var unsafeModulePathChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeModulePath(modulePath string) string {
	safe := unsafeModulePathChars.ReplaceAllString(modulePath, "_")
	safe = strings.Trim(safe, "_")
	if safe == "" || safe == "." || safe == ".." {
		return "module"
	}
	return safe
}
