package deps

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRestoreDependencyBackup_RestoresOfflineWithoutTidy verifies the
// hardened manual-restore semantics: the backup files are restored
// verbatim (exact byte restore), NO `go mod tidy` runs, the refresh
// is offline, and a pre-restore backup of the current files is saved.
func TestRestoreDependencyBackup_RestoresOfflineWithoutTidy(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	currentMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/current v1.0.0\n"
	currentSum := "example.com/current v1.0.0 h1:current=\n"
	restoredMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/restored v2.0.0\n"
	restoredSum := "example.com/restored v2.0.0 h1:restored=\n"
	writeFile(t, root, "go.mod", currentMod)
	writeFile(t, root, "go.sum", currentSum)

	backup, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	backup.ModFile.Content = restoredMod
	backup.SumFile.Content = restoredSum
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	info, err := defaultDependencyBackupStore().save(context, backup, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save dependency backup: %v", err)
	}

	var offline bool
	tidyCalled := false
	restored, err := restoreDependencyBackup(root, info.Name, defaultDependencyBackupLimit, dependencyOperation{
		runCommand: func(_ moduleContext, _ ...string) ([]byte, error) {
			tidyCalled = true
			return nil, nil
		},
		loadBackup: func(ctx moduleContext, name string) (*DependencyBackup, error) {
			return loadDependencyBackupResolved(ctx, name)
		},
		saveBackup: saveDependencyBackupResolvedWithRetention,
		restoreFiles: func(ctx moduleContext, snap *DependencySnapshot) error {
			return RestoreModuleFiles(ctx.Root, snap)
		},
		load: func(_ moduleContext, checkUpdates bool) ([]ModuleDependency, error) {
			if checkUpdates {
				t.Fatal("restore refresh must not check updates online")
			}
			offline = true
			return []ModuleDependency{{Path: "example.com/restored", Version: "v2.0.0"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("restoreDependencyBackup: %v", err)
	}

	if !offline {
		t.Fatal("expected restore refresh to run offline")
	}
	if tidyCalled {
		t.Fatal("restore must not run go mod tidy")
	}
	if len(restored.Dependencies) != 1 {
		t.Fatalf("dependencies = %+v, want restored dependency", restored.Dependencies)
	}
	gotMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read restored go.mod: %v", err)
	}
	if string(gotMod) != restoredMod {
		t.Fatalf("restored go.mod = %q, want exact %q", gotMod, restoredMod)
	}
	gotSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read restored go.sum: %v", err)
	}
	if string(gotSum) != restoredSum {
		t.Fatalf("restored go.sum = %q, want exact %q", gotSum, restoredSum)
	}
	backups, err := ListDependencyBackups(root)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	foundPreRestore := false
	for _, candidate := range backups {
		if candidate.Kind != DependencyBackupKindPreRestore {
			continue
		}
		preRestore, err := loadDependencyBackupResolved(context, candidate.Name)
		if err != nil {
			t.Fatalf("loadDependencyBackupResolved: %v", err)
		}
		if preRestore.Snapshot.ModFile.Content != currentMod || preRestore.Snapshot.SumFile.Content != currentSum {
			t.Fatalf("pre-restore snapshot = %+v, want current module bytes", preRestore.Snapshot)
		}
		foundPreRestore = true
	}
	if !foundPreRestore {
		t.Fatal("expected a pre-restore backup")
	}
}

// TestRollbackModuleDependencies_RefreshesOffline verifies rollback
// restores verbatim and refreshes offline without running
// `go mod tidy`.
func TestRollbackModuleDependencies_RefreshesOffline(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	snap, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	var offline bool
	tidyCalled := false
	_, err = rollbackModuleDependencies(root, snap, dependencyOperation{
		runCommand: func(_ moduleContext, _ ...string) ([]byte, error) {
			tidyCalled = true
			return nil, nil
		},
		restoreFiles: func(ctx moduleContext, snap *DependencySnapshot) error {
			return RestoreModuleFiles(ctx.Root, snap)
		},
		load: func(_ moduleContext, checkUpdates bool) ([]ModuleDependency, error) {
			if checkUpdates {
				t.Fatal("rollback refresh must not check updates online")
			}
			offline = true
			return []ModuleDependency{{Path: "example.com/restored", Version: "v1.0.0"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("rollbackModuleDependencies: %v", err)
	}

	if !offline {
		t.Fatal("expected rollback refresh to run offline")
	}
	if tidyCalled {
		t.Fatal("rollback must not run go mod tidy")
	}
}

func TestRestoreDependencyBackup_RestoreFilesFailureRestoresCurrentFiles(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	currentMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/current v1.0.0\n"
	currentSum := "example.com/current v1.0.0 h1:current=\n"
	restoredMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/restored v2.0.0\n"
	restoredSum := "example.com/restored v2.0.0 h1:restored=\n"
	writeFile(t, root, "go.mod", currentMod)
	writeFile(t, root, "go.sum", currentSum)

	backup, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	backup.ModFile.Content = restoredMod
	backup.SumFile.Content = restoredSum
	context, err := resolveModuleContext(root)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	info, err := defaultDependencyBackupStore().save(context, backup, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save dependency backup: %v", err)
	}

	restoreCalls := 0
	_, err = restoreDependencyBackup(root, info.Name, defaultDependencyBackupLimit, dependencyOperation{
		loadBackup: func(ctx moduleContext, name string) (*DependencyBackup, error) {
			return loadDependencyBackupResolved(ctx, name)
		},
		saveBackup: saveDependencyBackupResolvedWithRetention,
		restoreFiles: func(ctx moduleContext, snap *DependencySnapshot) error {
			restoreCalls++
			if restoreCalls == 1 {
				writeFile(t, ctx.Root, "go.mod", snap.ModFile.Content)
				writeFile(t, ctx.Root, "go.sum", "partial restore result\n")
				return errors.New("restore failed")
			}
			return RestoreModuleFiles(ctx.Root, snap)
		},
		runCommand: func(_ moduleContext, _ ...string) ([]byte, error) {
			t.Fatal("command must not run after restore error")
			return nil, nil
		},
		load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
			t.Fatal("loader must not run after restore error")
			return nil, nil
		},
	})

	if err == nil || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("error = %v, want restore error", err)
	}
	if restoreCalls != 2 {
		t.Fatalf("restore calls = %d, want 2", restoreCalls)
	}
	gotMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read restored go.mod: %v", err)
	}
	if string(gotMod) != currentMod {
		t.Fatalf("go.mod after restore failure = %q, want original bytes %q", gotMod, currentMod)
	}
	gotSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read restored go.sum: %v", err)
	}
	if string(gotSum) != currentSum {
		t.Fatalf("go.sum after restore failure = %q, want original bytes %q", gotSum, currentSum)
	}
	backups, err := ListDependencyBackups(root)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	foundPreRestore := false
	for _, candidate := range backups {
		if candidate.Kind == DependencyBackupKindPreRestore {
			foundPreRestore = true
			break
		}
	}
	if !foundPreRestore {
		t.Fatal("expected pre-restore backup to remain after restore failure")
	}
}
