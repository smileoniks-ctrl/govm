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
	"github.com/smileoniks-ctrl/govm/internal/prune"
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

func TestAppPruneSupportsDryRunWithoutMutation(t *testing.T) {
	var out bytes.Buffer
	pruned := false
	app := NewApp(Operations{
		PreviewPrune: func(context.Context) (prune.Result, error) {
			return prune.Result{
				Candidates: []prune.Candidate{{Path: "/versions/go1.23.0", Bytes: 2048}},
			}, nil
		},
		Prune: func(context.Context) (prune.Result, error) {
			pruned = true
			return prune.Result{}, nil
		},
	}, strings.NewReader(""), &out, &out)

	if !app.PruneVersions("--dry-run") {
		t.Fatal("PruneVersions returned false")
	}
	if pruned {
		t.Fatal("dry-run invoked prune operation")
	}
	if !strings.Contains(out.String(), "Would remove 1 object(s), 2048 bytes") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestAppPruneUsesYesFlag(t *testing.T) {
	var out bytes.Buffer
	pruned := false
	app := NewApp(Operations{
		PreviewPrune: func(context.Context) (prune.Result, error) {
			return prune.Result{
				Candidates: []prune.Candidate{{Path: "/tmp/archive.part", Bytes: 3}},
			}, nil
		},
		Prune: func(context.Context) (prune.Result, error) {
			pruned = true
			return prune.Result{
				Removed: []prune.Candidate{{Path: "/tmp/archive.part", Bytes: 3}},
			}, nil
		},
	}, strings.NewReader(""), &out, &out)

	if !app.PruneVersions("--yes") {
		t.Fatal("PruneVersions returned false")
	}
	if !pruned {
		t.Fatal("--yes did not invoke prune operation")
	}
	if !strings.Contains(out.String(), "Freed 3 bytes.") {
		t.Fatalf("output = %q", out.String())
	}
}
