package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func fixedDependencyBackupStore(now time.Time) dependencyBackupStore {
	return dependencyBackupStore{
		now: func() time.Time { return now },
	}
}

func TestPluralize(t *testing.T) {
	if Pluralize(1, "dep", "deps") != "dep" {
		t.Fatal("expected singular for n=1")
	}
	if Pluralize(0, "dep", "deps") != "deps" {
		t.Fatal("expected plural for n=0")
	}
	if Pluralize(5, "dep", "deps") != "deps" {
		t.Fatal("expected plural for n>1")
	}
}

func TestDirectDependencyUpdateEntriesFiltersAndMaps(t *testing.T) {
	deps := []ModuleDependency{
		{Path: "direct-updatable", Version: "v1.0.0", Latest: "v1.1.0"},
		{Path: "indirect-updatable", Version: "v2.0.0", Latest: "v2.1.0", Indirect: true},
		{Path: "direct-current", Version: "v3.0.0", Latest: "v3.0.0"},
		{Path: "direct-no-latest", Version: "v4.0.0"},
		{Path: "direct-error", Version: "v5.0.0", Latest: "v5.1.0", Error: "unavailable"},
	}

	entries := DirectDependencyUpdateEntries(deps)

	want := []DependencyUpdateEntry{{
		Path:       "direct-updatable",
		OldVersion: "v1.0.0",
		NewVersion: "v1.1.0",
	}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("DirectDependencyUpdateEntries() = %#v, want %#v", entries, want)
	}
}

func TestReadModulePath_QuotedDirective(t *testing.T) {
	requireGoToolchain(t)

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module \"example.com/app\"\n\ngo 1.26\n")

	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	if context.Path != "example.com/app" {
		t.Fatalf("module path = %q, want %q", context.Path, "example.com/app")
	}
}

func TestReadModulePath_MalformedOrMissingDirective(t *testing.T) {
	requireGoToolchain(t)

	tests := []struct {
		name  string
		goMod string
	}{
		{
			name:  "malformed",
			goMod: "module \"example.com/app\n\ngo 1.26\n",
		},
		{
			name:  "missing",
			goMod: "go 1.26\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "go.mod", tt.goMod)

			_, err := resolveModuleContext(dir)
			if err == nil {
				t.Fatal("expected resolveModuleContext to reject a go.mod without a valid module directive")
			}
			if !strings.Contains(err.Error(), "read go.mod: module path not found") {
				t.Fatalf("resolveModuleContext error = %q, want contextual module-path error", err)
			}
		})
	}
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

func TestSaveDependencyBackupUsesModulePathAndTimestamp(t *testing.T) {
	home := setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/acme/my-app\n\ngo 1.26\n")
	writeFile(t, dir, "go.sum", "github.com/x/y v1.0.0 h1:abc=\n")

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	snap.Updatable = []DependencyUpdateEntry{{Path: "github.com/x/y", OldVersion: "v1.0.0", NewVersion: "v1.1.0"}}

	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	store := fixedDependencyBackupStore(time.Date(2026, 7, 9, 12, 34, 56, 0, time.UTC))
	info, err := store.save(context, snap, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save dependency backup: %v", err)
	}

	wantPath := filepath.Join(home, ".govm", "deps_backup", "github.com_acme_my-app", "2026-07-09_12-34-56.json")
	if info.Path != wantPath {
		t.Fatalf("backup path = %q, want %q", info.Path, wantPath)
	}

	bytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	var backup DependencyBackup
	if err := json.Unmarshal(bytes, &backup); err != nil {
		t.Fatalf("unmarshal backup: %v", err)
	}
	if backup.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", backup.SchemaVersion)
	}
	if backup.ModulePath != "github.com/acme/my-app" {
		t.Fatalf("ModulePath = %q", backup.ModulePath)
	}
	if backup.Kind != DependencyBackupKindPreUpdate {
		t.Fatalf("Kind = %q", backup.Kind)
	}
	if backup.Snapshot == nil || backup.Snapshot.ModFile.Content == "" || len(backup.Snapshot.Updatable) != 1 {
		t.Fatalf("snapshot not persisted correctly: %+v", backup.Snapshot)
	}
}

func TestDependencyBackupProjectDirRejectsDotDotModulePath(t *testing.T) {
	home := setTestHome(t)

	dir, err := dependencyBackupProjectDir("..")
	if err != nil {
		t.Fatalf("dependencyBackupProjectDir: %v", err)
	}

	root := filepath.Join(home, ".govm", "deps_backup")
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("backup dir escaped root: root=%q dir=%q rel=%q", root, dir, rel)
	}
	if dir == filepath.Clean(filepath.Join(root, "..")) {
		t.Fatalf("backup dir collapsed to parent: %q", dir)
	}
}

func TestSaveDependencyBackupDoesNotOverwriteSameSecondBackup(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/acme/my-app\n\ngo 1.26\n")

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	store := fixedDependencyBackupStore(time.Date(2026, 7, 9, 12, 34, 56, 0, time.UTC))
	first, err := store.save(context, snap, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, err := store.save(context, snap, DependencyBackupKindPreRestore)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	if first.Path == second.Path {
		t.Fatalf("expected unique backup paths, both were %q", first.Path)
	}

	backups, err := ListDependencyBackups(dir)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d: %+v", len(backups), backups)
	}
}

func TestListDependencyBackupsSortsNewestFirst(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/acme/my-app\n\ngo 1.26\n")

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}

	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	for _, ts := range []time.Time{
		time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 9, 11, 0, 0, 0, time.UTC),
	} {
		store := fixedDependencyBackupStore(ts)
		if _, err := store.save(context, snap, DependencyBackupKindPreUpdate); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	backups, err := ListDependencyBackups(dir)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	if backups[0].Name != "2026-07-09_11-00-00.json" || backups[1].Name != "2026-07-09_10-00-00.json" {
		t.Fatalf("unexpected order: %+v", backups)
	}
}

func TestListDependencyBackupsSkipsInvalidBackupFiles(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/acme/my-app\n\ngo 1.26\n")

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	store := fixedDependencyBackupStore(time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	info, err := store.save(context, snap, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save dependency backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(info.Path), "broken.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write broken backup: %v", err)
	}

	backups, err := ListDependencyBackups(dir)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 valid backup, got %d: %+v", len(backups), backups)
	}
	if backups[0].Name != info.Name {
		t.Fatalf("expected valid backup %q, got %q", info.Name, backups[0].Name)
	}
}

func TestLoadDependencyBackupRejectsDifferentModule(t *testing.T) {
	home := setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/acme/current\n\ngo 1.26\n")
	backupDir := filepath.Join(home, ".govm", "deps_backup", "github.com_acme_current")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	backup := DependencyBackup{
		SchemaVersion: 1,
		CreatedAt:     time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
		ModulePath:    "github.com/acme/other",
		Kind:          DependencyBackupKindPreUpdate,
		Snapshot:      &DependencySnapshot{},
	}
	bytes, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("marshal backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "bad.json"), bytes, 0644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	_, err = loadDependencyBackupResolved(context, "bad.json")
	if err == nil || !strings.Contains(err.Error(), "belongs to module") {
		t.Fatalf("expected module mismatch error, got %v", err)
	}
}

func TestDependencyBackups_RemainAvailableAfterQuotedModuleIsNormalized(t *testing.T) {
	_ = setTestHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module \"github.com/acme/my-app\"\n\ngo 1.26\n")

	snap, err := SnapshotModuleFiles(dir)
	if err != nil {
		t.Fatalf("SnapshotModuleFiles: %v", err)
	}
	context, err := resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	info, err := defaultDependencyBackupStore().save(context, snap, DependencyBackupKindPreUpdate)
	if err != nil {
		t.Fatalf("save dependency backup: %v", err)
	}

	writeFile(t, dir, "go.mod", "module github.com/acme/my-app\n\ngo 1.26\n")

	backups, err := ListDependencyBackups(dir)
	if err != nil {
		t.Fatalf("ListDependencyBackups: %v", err)
	}
	if len(backups) != 1 || backups[0].Name != info.Name {
		t.Fatalf("ListDependencyBackups = %+v, want backup %q", backups, info.Name)
	}

	context, err = resolveModuleContext(dir)
	if err != nil {
		t.Fatalf("resolveModuleContext: %v", err)
	}
	backup, err := loadDependencyBackupResolved(context, info.Name)
	if err != nil {
		t.Fatalf("loadDependencyBackupResolved: %v", err)
	}
	if backup.ModulePath != "github.com/acme/my-app" {
		t.Fatalf("loaded ModulePath = %q, want %q", backup.ModulePath, "github.com/acme/my-app")
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

	rolled, err := RollbackModuleDependencies(dir, snap)
	if err != nil {
		t.Fatalf("RollbackModuleDependencies: %v", err)
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
		t.Fatal("expected snapshot to be propagated in DependencyRollbackResult")
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
