package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/paths"
)

func TestFilesystemRegistryListSortsToolchains(t *testing.T) {
	home := t.TempDir()
	versionsDir := filepath.Join(home, ".govm", "versions")
	for _, version := range []string{"1.21.5", "1.22.0", "1.20.9"} {
		goPath := filepath.Join(versionsDir, "go"+version, "bin", "go")
		if err := os.MkdirAll(filepath.Dir(goPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goPath, []byte("go"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	registry := NewRegistry(&paths.Resolver{HomeDir: func() (string, error) {
		return home, nil
	}})
	toolchains, err := registry.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := []string{toolchains[0].Version, toolchains[1].Version, toolchains[2].Version}
	want := []string{"1.22.0", "1.21.5", "1.20.9"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("versions = %v, want %v", got, want)
		}
	}
}

func TestFilesystemRegistryFindNormalizesAndMatchesPrefix(t *testing.T) {
	home := t.TempDir()
	goPath := filepath.Join(home, ".govm", "versions", "go1.22.4", "bin", "go")
	if err := os.MkdirAll(filepath.Dir(goPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goPath, []byte("go"), 0o755); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(&paths.Resolver{HomeDir: func() (string, error) {
		return home, nil
	}})
	toolchain, err := registry.Find(context.Background(), "v1.22")
	if err != nil {
		t.Fatal(err)
	}
	if toolchain.Version != "1.22.4" {
		t.Fatalf("version = %q, want 1.22.4", toolchain.Version)
	}
}
