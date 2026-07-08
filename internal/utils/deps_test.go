package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestSnapshotModuleFiles_WithGoSum(t *testing.T) {
	dir := t.TempDir()
	wantMod := "module example.com/test\n\ngo 1.26\n\nrequire github.com/x/y v1.0.0\n"
	wantSum := "github.com/x/y v1.0.0 h1:abc=\n"
	writeFile(t, dir, "go.mod", wantMod)
	writeFile(t, dir, "go.sum", wantSum)

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	if !snap.ModFile.Exists {
		t.Fatal("expected ModFile.Exists to be true")
	}
	if snap.ModFile.Content != wantMod {
		t.Fatalf("ModFile.Content mismatch: got %q, want %q", snap.ModFile.Content, wantMod)
	}
	if !snap.SumFile.Exists {
		t.Fatal("expected SumFile.Exists to be true")
	}
	if snap.SumFile.Content != wantSum {
		t.Fatalf("SumFile.Content mismatch: got %q, want %q", snap.SumFile.Content, wantSum)
	}
}

func TestSnapshotModuleFiles_MissingGoSum(t *testing.T) {
	dir := t.TempDir()
	wantMod := "module example.com/test\n\ngo 1.26\n"
	writeFile(t, dir, "go.mod", wantMod)

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	if !snap.ModFile.Exists {
		t.Fatal("expected ModFile.Exists to be true")
	}
	if snap.SumFile.Exists {
		t.Fatal("expected SumFile.Exists to be false")
	}
	if snap.SumFile.Content != "" {
		t.Fatalf("expected empty SumFile.Content, got %q", snap.SumFile.Content)
	}
}

func TestSnapshotModuleFiles_MissingGoMod(t *testing.T) {
	dir := t.TempDir()
	_, err := SnapshotModuleFiles(dir)
	if err == nil {
		t.Fatal("expected error when go.mod is missing")
	}
}

func TestSnapshotModuleFiles_TrailingSeparator(t *testing.T) {
	dir := t.TempDir()
	wantMod := "module example.com/test\n\ngo 1.26\n"
	writeFile(t, dir, "go.mod", wantMod)

	withSep := dir + string(os.PathSeparator)
	snap, err := SnapshotModuleFiles(withSep)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles with trailing separator: %v", err)
	}
	if snap.ModFile.Content != wantMod {
		t.Fatalf("ModFile.Content mismatch: got %q, want %q", snap.ModFile.Content, wantMod)
	}
}

func TestRestoreModuleFiles_RestoresGoSumAndGoMod(t *testing.T) {
	dir := t.TempDir()
	originalMod := "module example.com/test\n\ngo 1.26\n\nrequire github.com/x/y v1.0.0\n"
	originalSum := "github.com/x/y v1.0.0 h1:abc=\n"
	writeFile(t, dir, "go.mod", originalMod)
	writeFile(t, dir, "go.sum", originalSum)

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	// Simulate go get changing both files.
	changedMod := "module example.com/test\n\ngo 1.26\n\nrequire github.com/x/y v2.0.0\n"
	changedSum := "github.com/x/y v2.0.0 h1:def=\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(changedMod), 0644); err != nil {
		t.Fatalf("overwrite go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte(changedSum), 0644); err != nil {
		t.Fatalf("overwrite go.sum: %v", err)
	}

	if err := RestoreModuleFiles(dir, snap); err != nil {
		t.Fatalf("RestoreModuleFiles: %v", err)
	}

	gotMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if string(gotMod) != originalMod {
		t.Fatalf("go.mod not restored: got %q, want %q", gotMod, originalMod)
	}
	gotSum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		t.Fatalf("read go.sum: %v", err)
	}
	if string(gotSum) != originalSum {
		t.Fatalf("go.sum not restored: got %q, want %q", gotSum, originalSum)
	}
}

func TestRestoreModuleFiles_RemovesGoSumWhenOriginallyMissing(t *testing.T) {
	dir := t.TempDir()
	originalMod := "module example.com/test\n\ngo 1.26\n"
	writeFile(t, dir, "go.mod", originalMod)

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	// Simulate go get creating a go.sum.
	createdSum := "github.com/x/y v1.0.0 h1:abc=\n"
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte(createdSum), 0644); err != nil {
		t.Fatalf("create go.sum: %v", err)
	}

	if err := RestoreModuleFiles(dir, snap); err != nil {
		t.Fatalf("RestoreModuleFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("expected go.sum to be removed, stat err: %v", err)
	}
}

func TestRollbackModuleDependenciesTidiesRestoredFiles(t *testing.T) {
	dir := t.TempDir()

	// Snapshot represents a clean state without any require blocks.
	// go.sum carries stale entries that should not be there after tidy.
	originalMod := "module example.com/rollback-tidy\n\ngo 1.20\n"
	originalSum := strings.Join([]string{
		"github.com/sahilm/fuzzy v0.1.3 h1:juByESSS32nVD81vr6tHmKmA/8zde7gE+x5CLxrzXPU=",
		"github.com/sahilm/fuzzy v0.1.3/go.mod h1:au6//VbVSqu6DFrkL2CfjlJ5iURpNCPeE+1GwY3XsT8=",
		"",
	}, "\n")
	writeFile(t, dir, "go.mod", originalMod)
	writeFile(t, dir, "go.sum", originalSum)

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	// Simulate go get mutating the files; then restore the snapshot
	// so we end up with go.sum holding stale entries that tidy
	// must drop on rollback.
	changedMod := "module example.com/rollback-tidy\n\ngo 1.20\n\nrequire github.com/x/y v2.0.0\n"
	changedSum := "github.com/x/y v2.0.0 h1:def=\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(changedMod), 0644); err != nil {
		t.Fatalf("overwrite go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte(changedSum), 0644); err != nil {
		t.Fatalf("overwrite go.sum: %v", err)
	}
	if err := RestoreModuleFiles(dir, snap); err != nil {
		t.Fatalf("RestoreModuleFiles: %v", err)
	}

	// Sanity check: after restoring, the stale sum should still be
	// present so the test really exercises go mod tidy.
	beforeSum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		t.Fatalf("read go.sum before rollback: %v", err)
	}
	if !strings.Contains(string(beforeSum), "github.com/sahilm/fuzzy") {
		t.Fatalf("precondition failed: expected stale entry in go.sum, got %q", beforeSum)
	}

	cmd := RollbackModuleDependencies(dir, snap)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd")
	}
	msg := cmd()
	rolled, ok := msg.(DependenciesRolledBackMsg)
	if !ok {
		t.Fatalf("expected DependenciesRolledBackMsg, got %T: %+v", msg, msg)
	}

	afterSum, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		// go mod tidy may delete the file if there are no dependencies;
		// treat that as success.
		if !os.IsNotExist(err) {
			t.Fatalf("read go.sum after rollback: %v", err)
		}
	} else if strings.Contains(string(afterSum), "github.com/sahilm/fuzzy") {
		t.Fatalf("expected go mod tidy to drop stale entries, got %q", afterSum)
	}

	if rolled.Snapshot == nil {
		t.Fatal("expected snapshot to be propagated in DependenciesRolledBackMsg")
	}
}

func TestTrimOutput_LongOutput(t *testing.T) {
	lines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	out := trimOutput(strings.Join(lines, "\n"))

	if !strings.Contains(out, "more lines") {
		t.Fatalf("expected truncation marker, got: %s", out)
	}
	for i := 0; i < 8; i++ {
		if !strings.Contains(out, fmt.Sprintf("line %d", i)) {
			t.Fatalf("expected line %d in trimmed output, got: %s", i, out)
		}
	}
}

func TestTrimOutput_ShortOutput(t *testing.T) {
	out := trimOutput("line1\nline2")
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("expected short output unchanged, got: %s", out)
	}
	if strings.Contains(out, "more lines") {
		t.Fatalf("short output should not have truncation marker, got: %s", out)
	}
}

// requireGoToolchain skips the test when the `go` toolchain is not
// available on PATH. ResolveModuleRoot depends on `go env GOMOD`,
// so the test cannot run without it. We do not need network access:
// `go env GOMOD` is a local command.
func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not available: %v", err)
	}
}

// evalSymlinks is a small helper that evaluates symlinks on a path
// and fails the test on error. It is used to compare module roots
// resiliently on systems where t.TempDir() may return a path that
// traverses a symlink (e.g. /var -> /private/var on macOS).
func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks %s: %v", path, err)
	}
	return resolved
}

func TestResolveModuleRoot_HappyPath(t *testing.T) {
	requireGoToolchain(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.26\n")

	root, err := ResolveModuleRoot(dir)
	if err != nil {
		t.Fatalf("ResolveModuleRoot: %v", err)
	}

	want := evalSymlinks(t, dir)
	got := evalSymlinks(t, root)
	if got != want {
		t.Fatalf("ResolveModuleRoot returned %q, want %q", got, want)
	}
}

func TestResolveModuleRoot_FromSubfolder(t *testing.T) {
	requireGoToolchain(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n\ngo 1.26\n")

	subdir := filepath.Join(dir, "internal", "subpkg")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	root, err := ResolveModuleRoot(subdir)
	if err != nil {
		t.Fatalf("ResolveModuleRoot: %v", err)
	}

	want := evalSymlinks(t, dir)
	got := evalSymlinks(t, root)
	if got != want {
		t.Fatalf("ResolveModuleRoot returned %q, want %q", got, want)
	}
}

func TestResolveModuleRoot_NotInModule(t *testing.T) {
	requireGoToolchain(t)

	// t.TempDir() lives under os.TempDir() (e.g. /tmp or
	// /var/folders/.../T/...) which is not inside any Go module, so
	// `go env GOMOD` should report that the directory is not in a
	// module.
	dir := t.TempDir()

	_, err := ResolveModuleRoot(dir)
	if err == nil {
		t.Fatal("expected an error for a directory that is not in a Go module")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q must mention startDir %q", err, dir)
	}
	if !strings.Contains(err.Error(), "not in a Go module") {
		t.Fatalf("error %q must mention that the startDir is not in a Go module", err)
	}
}

func TestSnapshotModuleFiles_FindsGoModFromSubfolder(t *testing.T) {
	requireGoToolchain(t)

	dir := t.TempDir()
	wantMod := "module example.com/test\n\ngo 1.26\n"
	writeFile(t, dir, "go.mod", wantMod)

	subdir := filepath.Join(dir, "pkg", "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	// SnapshotModuleFiles requires a real module directory. The fix
	// for the bug is that callers resolve the module root first via
	// ResolveModuleRoot and then pass the resolved root into
	// SnapshotModuleFiles, so a subfolder as the input no longer
	// hides the go.mod.
	root, err := ResolveModuleRoot(subdir)
	if err != nil {
		t.Fatalf("ResolveModuleRoot: %v", err)
	}

	snap, err := SnapshotModuleFiles(root)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	if !snap.ModFile.Exists {
		t.Fatal("expected ModFile.Exists to be true")
	}
	if snap.ModFile.Content != wantMod {
		t.Fatalf("ModFile.Content mismatch: got %q, want %q", snap.ModFile.Content, wantMod)
	}
}
