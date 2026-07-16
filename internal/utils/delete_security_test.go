package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteVersionOnlyRemovesDirectVersionChild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	versionsDir := filepath.Join(home, ".govm", "versions")
	externalSentinel := filepath.Join(home, "sentinel")
	if err := os.MkdirAll(externalSentinel, 0o755); err != nil {
		t.Fatal(err)
	}

	msg := DeleteVersion(GoVersion{
		Version:   "1.24.0",
		Path:      externalSentinel,
		Installed: true,
	})()
	if _, ok := msg.(ErrMsg); !ok {
		t.Fatalf("DeleteVersion() = %T, want ErrMsg", msg)
	}
	if _, err := os.Stat(externalSentinel); err != nil {
		t.Fatalf("external sentinel was removed: %v", err)
	}

	versionPath := filepath.Join(versionsDir, "go1.24.0")
	if err := os.MkdirAll(versionPath, 0o755); err != nil {
		t.Fatal(err)
	}

	msg = DeleteVersion(GoVersion{
		Version:   "1.24.0",
		Path:      versionPath,
		Installed: true,
	})()
	if _, ok := msg.(DeleteCompleteMsg); !ok {
		t.Fatalf("DeleteVersion() = %T, want DeleteCompleteMsg", msg)
	}
	if _, err := os.Stat(versionPath); !os.IsNotExist(err) {
		t.Fatalf("legitimate version child still exists: %v", err)
	}
}
