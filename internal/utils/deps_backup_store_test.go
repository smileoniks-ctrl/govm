package utils

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestDependencyBackupStore_SaveUsesInjectedClockWithoutGlobalState(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	now := time.Date(2026, 7, 10, 12, 34, 56, 0, time.UTC)
	store := dependencyBackupStore{now: func() time.Time { return now }}
	info, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if !info.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", info.CreatedAt, now)
	}
}

func TestDependencyBackupStore_SaveCreatesUniqueNamesForSameTimestamp(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	store := dependencyBackupStore{
		now: func() time.Time {
			return time.Date(2026, 7, 10, 12, 34, 56, 0, time.UTC)
		},
	}
	first, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreRestore)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	if first.Name == second.Name {
		t.Fatalf("backup names collide: %q", first.Name)
	}
}

func TestDependencyBackupStore_SaveDoesNotPublishPartialFileOnWriteFailure(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	store := dependencyBackupStore{
		now:   time.Now,
		write: func(*os.File, []byte) (int, error) { return 0, errors.New("injected write failure") },
	}
	if _, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate); err == nil {
		t.Fatal("save succeeded, want injected write failure")
	}

	dir, err := dependencyBackupProjectDir(context.Path)
	if err != nil {
		t.Fatalf("dependencyBackupProjectDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup directory: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			t.Fatalf("published incomplete backup %q", entry.Name())
		}
	}
}

func TestDependencyBackupStore_SaveDoesNotPublishPartialFileOnShortWrite(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	store := dependencyBackupStore{
		now: time.Now,
		write: func(file *os.File, bytes []byte) (int, error) {
			return file.Write(bytes[:len(bytes)-1])
		},
	}
	if _, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("save error = %v, want io.ErrShortWrite", err)
	}
	assertDependencyBackupDirectoryEmpty(t, context.Path)
}

func TestDependencyBackupStore_SaveDoesNotPublishPartialFileOnSyncFailure(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	store := dependencyBackupStore{
		now:  time.Now,
		sync: func(*os.File) error { return errors.New("injected sync failure") },
	}
	if _, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate); err == nil {
		t.Fatal("save succeeded, want injected sync failure")
	}
	assertDependencyBackupDirectoryEmpty(t, context.Path)
}

func TestDependencyBackupStore_SaveDoesNotPublishPartialFileOnCloseFailure(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}

	store := dependencyBackupStore{
		now:   time.Now,
		close: func(*os.File) error { return errors.New("injected close failure") },
	}
	if _, err := store.save(context, &DependencySnapshot{}, DependencyBackupKindPreUpdate); err == nil {
		t.Fatal("save succeeded, want injected close failure")
	}
	assertDependencyBackupDirectoryEmpty(t, context.Path)
}

func assertDependencyBackupDirectoryEmpty(t *testing.T, modulePath string) {
	t.Helper()
	dir, err := dependencyBackupProjectDir(modulePath)
	if err != nil {
		t.Fatalf("dependencyBackupProjectDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("backup directory entries = %v, want none", entries)
	}
}

func TestUpdateModuleDependencies_ResolvesContextOnce(t *testing.T) {
	context := moduleContext{Root: t.TempDir(), Path: "example.com/app"}
	writeFile(t, context.Root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	resolves := 0
	operation := dependencyOperation{
		resolveContext: func(string) (moduleContext, error) {
			resolves++
			return context, nil
		},
		saveBackup: func(moduleContext, *DependencySnapshot, string) (DependencyBackupInfo, error) {
			return DependencyBackupInfo{}, nil
		},
		runCommand: func(string, ...string) ([]byte, error) { return nil, nil },
		load:       func(string, bool) tea.Msg { return DependenciesMsg{} },
	}

	msg := updateModuleDependencies(".", []ModuleDependency{{Path: "example.com/dep", Version: "v1.0.0", Latest: "v1.1.0"}}, operation)()
	if _, ok := msg.(DependenciesUpdatedMsg); !ok {
		t.Fatalf("update result = %T, want DependenciesUpdatedMsg", msg)
	}
	if resolves != 1 {
		t.Fatalf("context resolves = %d, want 1", resolves)
	}
}

func TestRestoreDependencyBackup_ResolvesContextOnce(t *testing.T) {
	context := moduleContext{Root: t.TempDir(), Path: "example.com/app"}
	writeFile(t, context.Root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	resolves := 0
	backup := &DependencyBackup{
		CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
		Snapshot:  &DependencySnapshot{ModFile: ModuleFileSnapshot{Exists: true, Content: "module example.com/app\n\ngo 1.26\n"}},
	}
	operation := dependencyOperation{
		resolveContext: func(string) (moduleContext, error) {
			resolves++
			return context, nil
		},
		loadBackup: func(moduleContext, string) (*DependencyBackup, error) { return backup, nil },
		saveBackup: func(moduleContext, *DependencySnapshot, string) (DependencyBackupInfo, error) {
			return DependencyBackupInfo{}, nil
		},
		restoreFiles: func(string, *DependencySnapshot) error { return nil },
		runCommand:   func(string, ...string) ([]byte, error) { return nil, nil },
		load:         func(string, bool) tea.Msg { return DependenciesMsg{} },
	}

	msg := restoreDependencyBackup(".", "saved.json", operation)()
	if _, ok := msg.(DependenciesRestoredMsg); !ok {
		t.Fatalf("restore result = %T, want DependenciesRestoredMsg", msg)
	}
	if resolves != 1 {
		t.Fatalf("context resolves = %d, want 1", resolves)
	}
}
