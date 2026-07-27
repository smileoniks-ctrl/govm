package model

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/prune"
)

func TestInstalledPrunePreviewAndConfirmation(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	m.previewPrune = func(context.Context) (prune.Result, error) {
		return prune.Result{
			Candidates: []prune.Candidate{{
				Path:    "/versions/go1.23.0",
				Version: "1.23.0",
				Bytes:   1024,
				Kind:    prune.CandidateVersion,
			}},
		}, nil
	}
	m.runPrune = func(context.Context) (prune.Result, error) {
		return prune.Result{
			Removed: []prune.Candidate{{
				Path:    "/versions/go1.23.0",
				Version: "1.23.0",
				Bytes:   1024,
				Kind:    prune.CandidateVersion,
			}},
		}, nil
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
	m = updated.(Model)
	if m.Prune.phase != prunePhasePreviewing {
		t.Fatal("expected prune preview to start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.Prune.phase != prunePhaseConfirming {
		t.Fatal("expected prune confirmation")
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
	m = updated.(Model)
	if m.Prune.phase != prunePhaseRunning {
		t.Fatal("expected prune operation to start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.Prune.phase == prunePhaseRunning {
		t.Fatal("expected prune operation to finish")
	}
	if m.Status.Kind() != "success" {
		t.Fatalf("status kind = %q, want success", m.Status.Kind())
	}
}

func TestPruneCommandsApplyOperationDeadlines(t *testing.T) {
	m := newTestModel(t)

	var preview, run, usage time.Duration
	m.previewPrune = func(ctx context.Context) (prune.Result, error) {
		preview = budget(t, ctx)
		return prune.Result{}, nil
	}
	m.runPrune = func(ctx context.Context) (prune.Result, error) {
		run = budget(t, ctx)
		return prune.Result{}, nil
	}
	m.diskUsage = func(ctx context.Context) (prune.Summary, error) {
		usage = budget(t, ctx)
		return prune.Summary{}, nil
	}

	m.previewPruneCmd()()
	m.pruneCmd()()
	m.diskUsageCmd()()

	for _, tc := range []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"preview", preview, prunePreviewTimeout},
		{"prune", run, pruneTimeout},
		{"disk usage", usage, diskUsageTimeout},
	} {
		if tc.got <= 0 || tc.got > tc.want {
			t.Errorf("%s budget = %v, want within %v", tc.name, tc.got, tc.want)
		}
	}
}

func budget(t *testing.T, ctx context.Context) time.Duration {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("prune operation received a context without a deadline")
	}
	return time.Until(deadline)
}

func TestDiskUsageUpdatesInstalledSizeColumn(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(diskUsageMsg{
		Summary: prune.Summary{
			InstalledBytes: 4096,
			VersionBytes:   map[string]int64{"1.24.4": 2048},
		},
	})
	got := updated.(Model)
	row := got.projection.installedModel().Rows()[0]
	if row[2] != "2.0 KiB" {
		t.Fatalf("size column = %q, want 2.0 KiB", row[2])
	}
}
