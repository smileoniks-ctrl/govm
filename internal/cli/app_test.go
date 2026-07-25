package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

func TestAppDeleteReadsInjectedConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	versions := filepath.Join(home, ".govm", "versions")
	versionDir := filepath.Join(versions, "go1.24.0")
	if err := os.MkdirAll(filepath.Join(versionDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "bin", "go"), []byte("go"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	app := NewApp(Operations{
		Resolver: paths.New(),
		Delete: func(context.Context, string) (lifecycle.DeletionResult, error) {
			return lifecycle.DeletionResult{Version: "1.24.0"}, nil
		},
	}, strings.NewReader("y\n"), &out, &out)
	app.DeleteVersion("1.24.0")

	if !strings.Contains(out.String(), "Successfully deleted Go 1.24.0") {
		t.Fatalf("output = %q", out.String())
	}
}
