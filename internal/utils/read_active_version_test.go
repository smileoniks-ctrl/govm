package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadActiveVersion_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_version")
	if err := os.WriteFile(path, []byte("1.22.0"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := readActiveVersion(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.22.0" {
		t.Errorf("got %q, want %q", got, "1.22.0")
	}
}

func TestReadActiveVersion_MissingFileReturnsEmptyNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_version") // does not exist

	got, err := readActiveVersion(path)
	if err != nil {
		t.Fatalf("missing file should not return error, got: %v", err)
	}
	if got != "" {
		t.Errorf("missing file should yield empty version, got %q", got)
	}
}

// TestReadActiveVersion_PermissionErrorSurfaces guards bug (I): before
// the refactor, any ReadFile failure (including permission denied) was
// swallowed and the orchestrator silently fell back to the system go.
// Now non-Existence errors propagate so a broken state is visible.
func TestReadActiveVersion_PermissionErrorSurfaces(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission bits; cannot test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "active_version")
	if err := os.WriteFile(path, []byte("1.22.0"), 0000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	got, err := readActiveVersion(path)
	if err == nil {
		t.Fatalf("permission-denied read should surface error, got version %q", got)
	}
	if got != "" {
		t.Errorf("error case should return empty version, got %q", got)
	}
	if !contains(err.Error(), "read active version file") {
		t.Errorf("error should carry the function prefix, got: %v", err)
	}
}

func TestReadActiveVersion_EmptyFileYieldsEmptyVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active_version")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := readActiveVersion(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty file should yield empty version, got %q", got)
	}
}

func TestReadActiveVersion_PreservesWhitespace(t *testing.T) {
	// The function performs no normalisation; callers own trimming if
	// they need to. Today the file is written without trailing
	// whitespace, but the contract should not silently mutate input.
	dir := t.TempDir()
	path := filepath.Join(dir, "active_version")
	if err := os.WriteFile(path, []byte("1.22.0\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := readActiveVersion(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.22.0\n" {
		t.Errorf("raw contents should be preserved, got %q", got)
	}
}
