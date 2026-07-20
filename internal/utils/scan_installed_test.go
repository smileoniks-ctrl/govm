package utils

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// makeVersionDir scaffolds a fake installed version directory with a
// bin/go binary so ScanInstalledVersions recognises it as installed.
func makeVersionDir(t *testing.T, root, version string, withBinary bool) {
	t.Helper()
	dir := filepath.Join(root, "go"+version)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if withBinary {
		goBin := filepath.Join(dir, "bin", "go")
		if err := os.WriteFile(goBin, []byte("#!/bin/sh\n"), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", goBin, err)
		}
	}
}

func TestScanInstalledVersions_OnlyCompleteInstallations(t *testing.T) {
	root := t.TempDir()
	// 1.22.0 is a working install.
	makeVersionDir(t, root, "1.22.0", true)
	// 1.21.5 is missing bin/go -> should be excluded by the bin/go check.
	makeVersionDir(t, root, "1.21.5", false)
	// 1.20.0 is a working install.
	makeVersionDir(t, root, "1.20.0", true)

	got, err := ScanInstalledVersions(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantVersions := []string{"1.22.0", "1.20.0"}
	gotVersions := make([]string, 0, len(got))
	for v := range got {
		gotVersions = append(gotVersions, v)
	}
	sort.Strings(gotVersions)
	sort.Strings(wantVersions)

	if len(gotVersions) != len(wantVersions) {
		t.Fatalf("got %d versions %v, want %d %v", len(gotVersions), gotVersions, len(wantVersions), wantVersions)
	}
	for i := range wantVersions {
		if gotVersions[i] != wantVersions[i] {
			t.Errorf("got[%d] = %q, want %q", i, gotVersions[i], wantVersions[i])
		}
		// Path must point at the version directory.
		wantPath := filepath.Join(root, "go"+wantVersions[i])
		if got[wantVersions[i]] != wantPath {
			t.Errorf("path for %q = %q, want %q", wantVersions[i], got[wantVersions[i]], wantPath)
		}
	}
}

func TestScanInstalledVersions_SkipsNonGoPrefixAndEmptyVersion(t *testing.T) {
	root := t.TempDir()
	// "go" with no version segment -> filtered by empty-version check.
	makeVersionDir(t, root, "", true)
	// A directory that does not start with "go" at all.
	if err := os.MkdirAll(filepath.Join(root, "cache", "bin"), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	// A regular file, not a directory.
	if err := os.WriteFile(filepath.Join(root, "go1.22.0.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// A valid install to confirm the scan still works.
	makeVersionDir(t, root, "1.22.0", true)

	got, err := ScanInstalledVersions(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if _, ok := got["1.22.0"]; !ok {
		t.Errorf("expected 1.22.0 in result, got %v", got)
	}
}

func TestScanInstalledVersions_EmptyDir(t *testing.T) {
	root := t.TempDir()
	got, err := ScanInstalledVersions(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestScanInstalledVersions_MissingDir(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does", "not", "exist")

	_, err := ScanInstalledVersions(missing)
	if err == nil {
		t.Fatalf("expected error for missing directory")
	}
	// Bug (II) fix: ReadDir errors now surface with the function prefix.
	if !contains(err.Error(), "scan installed versions") {
		t.Errorf("error should carry the function prefix, got: %v", err)
	}
}

// contains is a tiny helper to avoid pulling "strings" into this test
// file; the error wrapping contract is verified by substring match.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
