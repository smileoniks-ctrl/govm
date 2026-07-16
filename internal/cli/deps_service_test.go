package cli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type fakeOps struct {
	listFn         func(string) ([]utils.ModuleDependency, error)
	checkFn        func(string) ([]utils.ModuleDependency, error)
	updateFn       func(string, []utils.DependencyUpdateEntry) (utils.DependenciesUpdatedMsg, error)
	runChecksFn    func(string) (utils.DependencyCheckResultMsg, error)
	rollbackFn     func(string, *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error)
	listBackupsFn  func(string) ([]utils.DependencyBackupInfo, error)
	restoreFn      func(string, string) (utils.DependenciesRestoredMsg, error)
	updateCalls    int
	runChecksCalls int
	rollbackCalls  int
	restoreCalls   int
}

func TestRunBackupsPrintsNewestBackups(t *testing.T) {
	svc, _, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) { return nil, nil },
		func(string, bool) (bool, error) { return true, nil },
	)
	svc.ListBackups = func(string) ([]utils.DependencyBackupInfo, error) {
		return []utils.DependencyBackupInfo{
			{
				Name:       "2026-07-09_12-00-00.json",
				CreatedAt:  time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
				ModulePath: "github.com/acme/app",
				Kind:       utils.DependencyBackupKindPreUpdate,
				Updated:    2,
			},
		}, nil
	}

	if err := svc.RunBackups(); err != nil {
		t.Fatalf("RunBackups: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"2026-07-09_12-00-00.json", "github.com/acme/app", "pre-update", "2 update"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRunRestoreUsesProvidedBackupName(t *testing.T) {
	confirmCalls := 0
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) { return nil, nil },
		func(string, bool) (bool, error) {
			confirmCalls++
			return true, nil
		},
	)
	svc.RestoreBackup = ops.RestoreBackup
	ops.restoreFn = func(dir, name string) (utils.DependenciesRestoredMsg, error) {
		if name != "2026-07-09_12-00-00.json" {
			t.Fatalf("restore name = %q", name)
		}
		return utils.DependenciesRestoredMsg{
			BackupName:    name,
			BackupCreated: time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
		}, nil
	}

	if err := svc.RunRestore("2026-07-09_12-00-00.json"); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if confirmCalls != 1 {
		t.Fatalf("expected one confirmation, got %d", confirmCalls)
	}
	if ops.restoreCalls != 1 {
		t.Fatalf("expected one restore call, got %d", ops.restoreCalls)
	}
	if !strings.Contains(stdout.String(), "Restored dependencies") {
		t.Fatalf("expected restore success, got:\n%s", stdout.String())
	}
}

func (f *fakeOps) ListDeps(dir string) ([]utils.ModuleDependency, error) {
	return f.listFn(dir)
}
func (f *fakeOps) CheckDeps(dir string) ([]utils.ModuleDependency, error) {
	return f.checkFn(dir)
}
func (f *fakeOps) Update(dir string, entries []utils.DependencyUpdateEntry) (utils.DependenciesUpdatedMsg, error) {
	f.updateCalls++
	return f.updateFn(dir, entries)
}
func (f *fakeOps) RunChecks(dir string) (utils.DependencyCheckResultMsg, error) {
	f.runChecksCalls++
	return f.runChecksFn(dir)
}
func (f *fakeOps) Rollback(dir string, snap *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
	f.rollbackCalls++
	return f.rollbackFn(dir, snap)
}
func (f *fakeOps) ListBackups(dir string) ([]utils.DependencyBackupInfo, error) {
	return f.listBackupsFn(dir)
}
func (f *fakeOps) RestoreBackup(dir, name string) (utils.DependenciesRestoredMsg, error) {
	f.restoreCalls++
	return f.restoreFn(dir, name)
}

func newFakeDeps(checkFn func(string) ([]utils.ModuleDependency, error), confirm func(string, bool) (bool, error)) (*DepsService, *fakeOps, *bytes.Buffer) {
	ops := &fakeOps{checkFn: checkFn}
	stdout := &bytes.Buffer{}
	svc := &DepsService{
		ModuleDir:     "/tmp/m",
		Stdout:        stdout,
		Stdin:         &bytes.Buffer{},
		Confirm:       confirm,
		ListDeps:      ops.ListDeps,
		CheckDeps:     ops.CheckDeps,
		Update:        ops.Update,
		RunChecks:     ops.RunChecks,
		Rollback:      ops.Rollback,
		ListBackups:   ops.ListBackups,
		RestoreBackup: ops.RestoreBackup,
	}
	ops.listFn = func(dir string) ([]utils.ModuleDependency, error) {
		return []utils.ModuleDependency{{Path: "x", Version: "v1.0.0"}}, nil
	}
	ops.updateFn = func(dir string, entries []utils.DependencyUpdateEntry) (utils.DependenciesUpdatedMsg, error) {
		return utils.DependenciesUpdatedMsg{
			Updated: 1,
			Snapshot: &utils.DependencySnapshot{
				Updatable: []utils.DependencyUpdateEntry{{Path: "x", OldVersion: "v1.0.0", NewVersion: "v1.1.0"}},
			},
		}, nil
	}
	ops.runChecksFn = func(dir string) (utils.DependencyCheckResultMsg, error) {
		return utils.DependencyCheckResultMsg{OK: true}, nil
	}
	ops.rollbackFn = func(dir string, snap *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
		return utils.DependenciesRolledBackMsg{Snapshot: snap}, nil
	}
	ops.listBackupsFn = func(dir string) ([]utils.DependencyBackupInfo, error) {
		return nil, nil
	}
	ops.restoreFn = func(dir, name string) (utils.DependenciesRestoredMsg, error) {
		return utils.DependenciesRestoredMsg{}, nil
	}
	return svc, ops, stdout
}

func TestRunListPrintsDependencies(t *testing.T) {
	svc, _, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) { return nil, nil },
		func(string, bool) (bool, error) { return true, nil },
	)
	svc.ListDeps = func(string) ([]utils.ModuleDependency, error) {
		return []utils.ModuleDependency{
			{Path: "github.com/d/x", Version: "v1.0.0"},
			{Path: "github.com/i/y", Version: "v0.5.0", Indirect: true},
		}, nil
	}

	if err := svc.RunList(); err != nil {
		t.Fatalf("RunList: %v", err)
	}
	want := "🔍 Reading module dependencies in /tmp/m...\n\n" +
		"  github.com/d/x\tv1.0.0\tdirect\n" +
		"  github.com/i/y\tv0.5.0\tindirect\n" +
		"\n✅ 1 direct, 1 indirect dependencies.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("RunList output mismatch (-want +got):\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestRunCheckShowsUpdates(t *testing.T) {
	svc, _, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
				{Path: "github.com/d/y", Version: "v1.0.0", Latest: "v1.0.0"},
			}, nil
		},
		func(string, bool) (bool, error) { return true, nil },
	)
	svc.CheckDeps = func(string) ([]utils.ModuleDependency, error) {
		return []utils.ModuleDependency{
			{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			{Path: "github.com/d/y", Version: "v1.0.0", Latest: "v1.0.0"},
		}, nil
	}

	if err := svc.RunCheck(); err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	want := "🔍 Checking available updates in /tmp/m...\n\n" +
		"  github.com/d/x\tv1.0.0 → v1.1.0\tupdate available\n" +
		"  github.com/d/y\tv1.0.0\tcurrent\n" +
		"\n📦 1 direct update(s) available.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("RunCheck output mismatch (-want +got):\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestRunUpdateNoDirectUpdates(t *testing.T) {
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.0.0"},
			}, nil
		},
		func(string, bool) (bool, error) {
			t.Fatal("Confirm should not be called when no updates")
			return false, nil
		},
	)

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if !strings.Contains(stdout.String(), "No direct dependency updates available") {
		t.Fatalf("expected message about no updates, got:\n%s", stdout.String())
	}
	if ops.updateCalls != 0 || ops.runChecksCalls != 0 || ops.rollbackCalls != 0 {
		t.Fatalf("expected no operations, got update=%d checks=%d rollback=%d", ops.updateCalls, ops.runChecksCalls, ops.rollbackCalls)
	}
}

func TestRunUpdateDeclineUpdate(t *testing.T) {
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		nil,
	)
	svc.Stdin = strings.NewReader("n\n")
	svc.Confirm = defaultConfirm(svc.Stdin, stdout)

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	want := "🔍 Checking available updates in /tmp/m...\n\n" +
		"⚠️  1 direct dependency will be updated:\n" +
		"  - github.com/d/x  v1.0.0 → v1.1.0\n" +
		"\nApply these updates? [Y/n]: 🛑 Update canceled.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("RunUpdate cancellation output mismatch (-want +got):\nwant:\n%q\ngot:\n%q", want, got)
	}
	if ops.updateCalls != 0 {
		t.Fatalf("expected Update to be skipped, got %d calls", ops.updateCalls)
	}
}

func TestRunUpdateAcceptUpdateDeclineChecks(t *testing.T) {
	confirmCalls := 0
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) {
			confirmCalls++
			return confirmCalls == 1, nil // accept update, decline checks
		},
	)

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Updated 1 direct dependency") {
		t.Fatalf("expected success message, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Checks skipped") {
		t.Fatalf("expected checks skipped message, got:\n%s", stdout.String())
	}
	if ops.updateCalls != 1 {
		t.Fatalf("expected Update to be called once, got %d", ops.updateCalls)
	}
	if ops.runChecksCalls != 0 {
		t.Fatalf("expected RunChecks to be skipped, got %d calls", ops.runChecksCalls)
	}
	if ops.rollbackCalls != 0 {
		t.Fatalf("expected Rollback to be skipped, got %d calls", ops.rollbackCalls)
	}
}

func TestRunUpdateRunChecksSuccess(t *testing.T) {
	confirmCalls := 0
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) { confirmCalls++; return true, nil },
	)

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Checks passed") {
		t.Fatalf("expected success message, got:\n%s", stdout.String())
	}
	if ops.updateCalls != 1 || ops.runChecksCalls != 1 || ops.rollbackCalls != 0 {
		t.Fatalf("unexpected call counts update=%d checks=%d rollback=%d", ops.updateCalls, ops.runChecksCalls, ops.rollbackCalls)
	}
}

func TestRunUpdateChecksFailAcceptRollback(t *testing.T) {
	confirmCalls := 0
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) { confirmCalls++; return true, nil },
	)
	ops.runChecksFn = func(string) (utils.DependencyCheckResultMsg, error) {
		return utils.DependencyCheckResultMsg{OK: false, Command: "go test ./...", Output: "FAIL: x"}, nil
	}

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Checks failed", "go test ./...", "FAIL: x", "Rolled back to pre-update state"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
	if ops.rollbackCalls != 1 {
		t.Fatalf("expected Rollback to be called once, got %d", ops.rollbackCalls)
	}
}

func TestRunUpdateChecksFailDeclineRollback(t *testing.T) {
	confirmCalls := 0
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) { confirmCalls++; return confirmCalls != 3, nil },
	)
	ops.runChecksFn = func(string) (utils.DependencyCheckResultMsg, error) {
		return utils.DependencyCheckResultMsg{OK: false, Command: "go test ./...", Output: "FAIL: x"}, nil
	}

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Update kept. Failed checks were not rolled back") {
		t.Fatalf("expected kept-updates message, got:\n%s", stdout.String())
	}
	if ops.rollbackCalls != 0 {
		t.Fatalf("expected Rollback to be skipped, got %d calls", ops.rollbackCalls)
	}
}

func TestRunUpdateCheckError(t *testing.T) {
	svc, ops, _ := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) { return nil, errors.New("boom") },
		func(string, bool) (bool, error) {
			t.Fatal("Confirm should not be called when check errors")
			return false, nil
		},
	)
	err := svc.RunUpdate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to surface, got: %v", err)
	}
	if ops.updateCalls != 0 {
		t.Fatalf("expected Update to be skipped, got %d", ops.updateCalls)
	}
}

func TestRunUpdateUpdateError(t *testing.T) {
	svc, ops, _ := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) { return true, nil },
	)
	ops.updateFn = func(_ string, entries []utils.DependencyUpdateEntry) (utils.DependenciesUpdatedMsg, error) {
		want := []utils.DependencyUpdateEntry{{
			Path:       "github.com/d/x",
			OldVersion: "v1.0.0",
			NewVersion: "v1.1.0",
		}}
		if !reflect.DeepEqual(entries, want) {
			t.Fatalf("update entries = %#v, want %#v", entries, want)
		}
		return utils.DependenciesUpdatedMsg{}, errors.New("go get failed")
	}
	if err := svc.RunUpdate(); err == nil {
		t.Fatal("expected error, got nil")
	}
	if ops.runChecksCalls != 0 || ops.rollbackCalls != 0 {
		t.Fatalf("expected no further calls, got checks=%d rollback=%d", ops.runChecksCalls, ops.rollbackCalls)
	}
}

func TestRunUpdateRollbackError(t *testing.T) {
	svc, ops, stdout := newFakeDeps(
		func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
			}, nil
		},
		func(string, bool) (bool, error) { return true, nil },
	)
	ops.runChecksFn = func(string) (utils.DependencyCheckResultMsg, error) {
		return utils.DependencyCheckResultMsg{OK: false, Command: "go test", Output: "boom"}, nil
	}
	ops.rollbackFn = func(string, *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
		return utils.DependenciesRolledBackMsg{}, errors.New("disk full")
	}
	if err := svc.RunUpdate(); err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(stdout.String(), "Checks failed") {
		t.Fatalf("expected failure context, got:\n%s", stdout.String())
	}
}

func TestDefaultConfirmReadsBufferedAnswersAcrossPrompts(t *testing.T) {
	stdin := strings.NewReader("y\nn\nn\n")
	stdout := &bytes.Buffer{}
	confirm := defaultConfirm(stdin, stdout)

	first, err := confirm("Apply updates?", true)
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	second, err := confirm("Run checks?", true)
	if err != nil {
		t.Fatalf("second confirm: %v", err)
	}
	third, err := confirm("Roll back?", true)
	if err != nil {
		t.Fatalf("third confirm: %v", err)
	}

	if first != true || second != false || third != false {
		t.Fatalf("expected true, false, false; got %t, %t, %t", first, second, third)
	}
}
