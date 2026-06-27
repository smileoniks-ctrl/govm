package utils

import (
	"fmt"
	"os"
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
