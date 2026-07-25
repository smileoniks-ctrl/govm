package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/paths"
)

func TestVersionScanner_ScanInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	versionsDir := filepath.Join(tmpDir, ".govm", "versions")

	resolver := &paths.Resolver{
		HomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	scanner := NewVersionScanner(resolver)

	t.Run("empty directory", func(t *testing.T) {
		if err := os.MkdirAll(versionsDir, 0755); err != nil {
			t.Fatalf("failed to create versions dir: %v", err)
		}

		installed, err := scanner.ScanInstalled(context.Background())
		if err != nil {
			t.Fatalf("ScanInstalled failed: %v", err)
		}

		if len(installed) != 0 {
			t.Errorf("expected empty map, got %d entries", len(installed))
		}
	})

	t.Run("with installed versions", func(t *testing.T) {
		version1Dir := filepath.Join(versionsDir, "go1.22.0")
		version2Dir := filepath.Join(versionsDir, "go1.21.5")

		for _, dir := range []string{version1Dir, version2Dir} {
			binDir := filepath.Join(dir, "bin")
			if err := os.MkdirAll(binDir, 0755); err != nil {
				t.Fatalf("failed to create bin dir: %v", err)
			}
			goBin := filepath.Join(binDir, "go")
			if err := os.WriteFile(goBin, []byte("fake go binary"), 0755); err != nil {
				t.Fatalf("failed to create go binary: %v", err)
			}
		}

		installed, err := scanner.ScanInstalled(context.Background())
		if err != nil {
			t.Fatalf("ScanInstalled failed: %v", err)
		}

		if len(installed) != 2 {
			t.Errorf("expected 2 entries, got %d", len(installed))
		}

		if path, ok := installed["1.22.0"]; !ok {
			t.Errorf("version 1.22.0 not found")
		} else if path != version1Dir {
			t.Errorf("expected path %s, got %s", version1Dir, path)
		}

		if path, ok := installed["1.21.5"]; !ok {
			t.Errorf("version 1.21.5 not found")
		} else if path != version2Dir {
			t.Errorf("expected path %s, got %s", version2Dir, path)
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		tmpDir2 := t.TempDir()
		resolver2 := &paths.Resolver{
			HomeDir: func() (string, error) {
				return tmpDir2, nil
			},
		}
		scanner2 := NewVersionScanner(resolver2)

		_, err := scanner2.ScanInstalled(context.Background())
		if err == nil {
			t.Error("expected error for missing directory, got nil")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := scanner.ScanInstalled(ctx)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("context timeout during scan", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		_, err := scanner.ScanInstalled(ctx)
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	})
}

func TestActiveReader_ReadActive(t *testing.T) {
	tmpDir := t.TempDir()
	activeFile := filepath.Join(tmpDir, ".govm", "active_version")

	if err := os.MkdirAll(filepath.Dir(activeFile), 0755); err != nil {
		t.Fatalf("failed to create .govm dir: %v", err)
	}

	resolver := &paths.Resolver{
		HomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	reader := NewActiveReader(resolver)

	t.Run("missing file", func(t *testing.T) {
		version, err := reader.ReadActive(context.Background())
		if err != nil {
			t.Fatalf("ReadActive failed: %v", err)
		}

		if version != "" {
			t.Errorf("expected empty version, got %s", version)
		}
	})

	t.Run("with active version", func(t *testing.T) {
		expectedVersion := "1.22.0"
		if err := os.WriteFile(activeFile, []byte(expectedVersion), 0644); err != nil {
			t.Fatalf("failed to write active version: %v", err)
		}

		version, err := reader.ReadActive(context.Background())
		if err != nil {
			t.Fatalf("ReadActive failed: %v", err)
		}

		if version != expectedVersion {
			t.Errorf("expected version %s, got %s", expectedVersion, version)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := reader.ReadActive(ctx)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

func TestActiveReader_GetFromPath(t *testing.T) {
	tmpDir := t.TempDir()

	resolver := &paths.Resolver{
		HomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	reader := NewActiveReader(resolver)

	t.Run("exec fallback", func(t *testing.T) {
		version := reader.GetFromPath(context.Background())

		if version != "" {
			t.Logf("got version from PATH: %s", version)
		}
	})
}
