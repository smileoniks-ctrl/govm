package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type fakeOps struct {
	listFn         func(string) ([]utils.ModuleDependency, error)
	checkFn        func(string) ([]utils.ModuleDependency, error)
	updateFn       func(string, []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error)
	runChecksFn    func(string) (utils.DependencyCheckResultMsg, error)
	rollbackFn     func(string, *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error)
	updateCalls    int
	runChecksCalls int
	rollbackCalls  int
}

func (f *fakeOps) ListDeps(dir string) ([]utils.ModuleDependency, error) {
	return f.listFn(dir)
}
func (f *fakeOps) CheckDeps(dir string) ([]utils.ModuleDependency, error) {
	return f.checkFn(dir)
}
func (f *fakeOps) Update(dir string, deps []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
	f.updateCalls++
	return f.updateFn(dir, deps)
}
func (f *fakeOps) RunChecks(dir string) (utils.DependencyCheckResultMsg, error) {
	f.runChecksCalls++
	return f.runChecksFn(dir)
}
func (f *fakeOps) Rollback(dir string, snap *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
	f.rollbackCalls++
	return f.rollbackFn(dir, snap)
}

func newFakeDeps(checkFn func(string) ([]utils.ModuleDependency, error), confirm func(string, bool) (bool, error)) (*DepsService, *fakeOps, *bytes.Buffer) {
	ops := &fakeOps{checkFn: checkFn}
	stdout := &bytes.Buffer{}
	svc := &DepsService{
		ModuleDir: "/tmp/m",
		Stdout:    stdout,
		Stdin:     &bytes.Buffer{},
		Confirm:   confirm,
		ListDeps:  ops.ListDeps,
		CheckDeps: ops.CheckDeps,
		Update:    ops.Update,
		RunChecks: ops.RunChecks,
		Rollback:  ops.Rollback,
	}
	ops.listFn = func(dir string) ([]utils.ModuleDependency, error) {
		return []utils.ModuleDependency{{Path: "x", Version: "v1.0.0"}}, nil
	}
	ops.updateFn = func(dir string, deps []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
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
	return svc, ops, stdout
}

func TestRunListPrintsDependencies(t *testing.T) {
	stdout := &bytes.Buffer{}
	svc := &DepsService{
		ModuleDir: "/tmp/m",
		Stdout:    stdout,
		Stdin:     &bytes.Buffer{},
		Confirm:   nil,
		ListDeps: func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0"},
				{Path: "github.com/i/y", Version: "v0.5.0", Indirect: true},
			}, nil
		},
		CheckDeps: func(string) ([]utils.ModuleDependency, error) { return nil, nil },
		Update: func(string, []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
			return utils.DependenciesUpdatedMsg{}, nil
		},
		RunChecks: func(string) (utils.DependencyCheckResultMsg, error) { return utils.DependencyCheckResultMsg{}, nil },
		Rollback: func(string, *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
			return utils.DependenciesRolledBackMsg{}, nil
		},
	}

	if err := svc.RunList(); err != nil {
		t.Fatalf("RunList: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"github.com/d/x", "v1.0.0", "direct", "github.com/i/y", "indirect", "1 direct, 1 indirect"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRunCheckShowsUpdates(t *testing.T) {
	stdout := &bytes.Buffer{}
	svc := &DepsService{
		ModuleDir: "/tmp/m",
		Stdout:    stdout,
		Stdin:     &bytes.Buffer{},
		Confirm:   nil,
		ListDeps:  func(string) ([]utils.ModuleDependency, error) { return nil, nil },
		CheckDeps: func(string) ([]utils.ModuleDependency, error) {
			return []utils.ModuleDependency{
				{Path: "github.com/d/x", Version: "v1.0.0", Latest: "v1.1.0"},
				{Path: "github.com/d/y", Version: "v1.0.0", Latest: "v1.0.0"},
			}, nil
		},
		Update: func(string, []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
			return utils.DependenciesUpdatedMsg{}, nil
		},
		RunChecks: func(string) (utils.DependencyCheckResultMsg, error) { return utils.DependencyCheckResultMsg{}, nil },
		Rollback: func(string, *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
			return utils.DependenciesRolledBackMsg{}, nil
		},
	}

	if err := svc.RunCheck(); err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"v1.0.0 → v1.1.0", "update available", "v1.0.0\tcurrent", "1 direct update"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
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
		func(string, bool) (bool, error) { return false, nil },
	)

	if err := svc.RunUpdate(); err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Update canceled") {
		t.Fatalf("expected cancellation message, got:\n%s", stdout.String())
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
	ops.updateFn = func(string, []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
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
