package utils

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRestoreDependencyBackup_RefreshesOfflineAndSavesPreRestoreBackup(t *testing.T) {
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
	info, err := SaveDependencyBackup(root, backup, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("SaveDependencyBackup: %v", err)
	}

	var offline bool
	msg := restoreDependencyBackup(root, info.Name, dependencyOperation{
		resolveRoot: func(string) (string, error) { return root, nil },
		runCommand: func(_ string, args ...string) ([]byte, error) {
			if strings.Join(args, " ") != "mod tidy" {
				t.Fatalf("command = %q, want %q", args, "mod tidy")
			}
			return nil, nil
		},
		load: func(_ string, checkUpdates bool) tea.Msg {
			if checkUpdates {
				t.Fatal("restore refresh must not check updates online")
			}
			offline = true
			return DependenciesMsg{{Path: "example.com/restored", Version: "v2.0.0"}}
		},
	})()

	if !offline {
		t.Fatal("expected restore refresh to run offline")
	}
	restored, ok := msg.(DependenciesRestoredMsg)
	if !ok {
		t.Fatalf("message = %T, want DependenciesRestoredMsg", msg)
	}
	if len(restored.Dependencies) != 1 {
		t.Fatalf("dependencies = %+v, want restored dependency", restored.Dependencies)
	}
	gotMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read restored go.mod: %v", err)
	}
	if string(gotMod) != restoredMod {
		t.Fatalf("restored go.mod = %q, want %q", gotMod, restoredMod)
	}
	gotSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read restored go.sum: %v", err)
	}
	if string(gotSum) != restoredSum {
		t.Fatalf("restored go.sum = %q, want %q", gotSum, restoredSum)
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
		preRestore, err := LoadDependencyBackup(root, candidate.Name)
		if err != nil {
			t.Fatalf("LoadDependencyBackup: %v", err)
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

func TestRollbackModuleDependencies_RefreshesOffline(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	snap, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	var offline bool
	msg := rollbackModuleDependencies(root, snap, dependencyOperation{
		resolveRoot: func(string) (string, error) { return root, nil },
		runCommand: func(_ string, args ...string) ([]byte, error) {
			if strings.Join(args, " ") != "mod tidy" {
				t.Fatalf("command = %q, want %q", args, "mod tidy")
			}
			return nil, nil
		},
		load: func(_ string, checkUpdates bool) tea.Msg {
			if checkUpdates {
				t.Fatal("rollback refresh must not check updates online")
			}
			offline = true
			return DependenciesMsg{{Path: "example.com/restored", Version: "v1.0.0"}}
		},
	})()

	if !offline {
		t.Fatal("expected rollback refresh to run offline")
	}
	if _, ok := msg.(DependenciesRolledBackMsg); !ok {
		t.Fatalf("message = %T, want DependenciesRolledBackMsg", msg)
	}
}

func TestRestoreDependencyBackup_TidyFailureHasContext(t *testing.T) {
	setTestHome(t)
	root := t.TempDir()
	currentMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/current v1.0.0\n"
	currentSum := "example.com/current v1.0.0 h1:current=\n"
	restoredMod := "module example.com/app\n\ngo 1.26\n\nrequire example.com/restored v2.0.0\n"
	restoredSum := "example.com/restored v2.0.0 h1:restored=\n"
	writeFile(t, root, "go.mod", currentMod)
	writeFile(t, root, "go.sum", currentSum)
	snap, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	snap.ModFile.Content = restoredMod
	snap.SumFile.Content = restoredSum
	info, err := SaveDependencyBackup(root, snap, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("SaveDependencyBackup: %v", err)
	}

	msg := restoreDependencyBackup(root, info.Name, dependencyOperation{
		resolveRoot: func(string) (string, error) { return root, nil },
		runCommand: func(_ string, args ...string) ([]byte, error) {
			if strings.Join(args, " ") != "mod tidy" {
				t.Fatalf("command = %q, want %q", args, "mod tidy")
			}
			writeFile(t, root, "go.mod", "partial tidy result\n")
			writeFile(t, root, "go.sum", "partial tidy result\n")
			return []byte("tidy output"), errors.New("tidy failed")
		},
		load: func(string, bool) tea.Msg { t.Fatal("loader must not run after tidy error"); return nil },
	})()

	errMsg, ok := msg.(DependencyErrMsg)
	if !ok || !strings.Contains(errMsg.Err.Error(), "restore go mod tidy failed: tidy output") {
		t.Fatalf("message = %+v, want contextual restore tidy error", msg)
	}
	gotMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read restored go.mod: %v", err)
	}
	if string(gotMod) != currentMod {
		t.Fatalf("go.mod after tidy failure = %q, want original bytes %q", gotMod, currentMod)
	}
	gotSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read restored go.sum: %v", err)
	}
	if string(gotSum) != currentSum {
		t.Fatalf("go.sum after tidy failure = %q, want original bytes %q", gotSum, currentSum)
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
		t.Fatal("expected pre-restore backup to remain after tidy failure")
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
	info, err := SaveDependencyBackup(root, backup, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("SaveDependencyBackup: %v", err)
	}

	restoreCalls := 0
	msg := restoreDependencyBackup(root, info.Name, dependencyOperation{
		resolveRoot: func(string) (string, error) { return root, nil },
		restoreFiles: func(moduleDir string, snap *DependencySnapshot) error {
			restoreCalls++
			if restoreCalls == 1 {
				writeFile(t, moduleDir, "go.mod", snap.ModFile.Content)
				writeFile(t, moduleDir, "go.sum", "partial restore result\n")
				return errors.New("restore failed")
			}
			return RestoreModuleFiles(moduleDir, snap)
		},
		runCommand: func(string, ...string) ([]byte, error) {
			t.Fatal("command must not run after restore error")
			return nil, nil
		},
		load: func(string, bool) tea.Msg { t.Fatal("loader must not run after restore error"); return nil },
	})()

	errMsg, ok := msg.(DependencyErrMsg)
	if !ok || !strings.Contains(errMsg.Err.Error(), "restore failed") {
		t.Fatalf("message = %+v, want restore error", msg)
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

func TestRollbackModuleDependencies_TidyFailureHasContext(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	snap, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	msg := rollbackModuleDependencies(root, snap, dependencyOperation{
		resolveRoot: func(string) (string, error) { return root, nil },
		runCommand:  func(string, ...string) ([]byte, error) { return []byte("tidy output"), errors.New("tidy failed") },
		load:        func(string, bool) tea.Msg { t.Fatal("loader must not run after tidy error"); return nil },
	})()

	errMsg, ok := msg.(DependencyErrMsg)
	if !ok || !strings.Contains(errMsg.Err.Error(), "rollback go mod tidy failed: tidy output") {
		t.Fatalf("message = %+v, want contextual rollback tidy error", msg)
	}
}
