package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindInstalledVersionRejectsTraversalQueries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	versionsDir := filepath.Join(home, ".govm", "versions")
	if err := os.MkdirAll(filepath.Join(versionsDir, "go1.24.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{
		"../../../..",
		"v../../../..",
		"go../../../..",
	} {
		t.Run(query, func(t *testing.T) {
			if _, err := findInstalledVersion(query); err == nil {
				t.Fatalf("findInstalledVersion(%q) resolved a traversal query", query)
			}
		})
	}
}
