package model

import (
	"context"
	"testing"

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
	m.prune = func(context.Context) (prune.Result, error) {
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
	if !m.PrunePreviewing {
		t.Fatal("expected prune preview to start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if !m.PruneConfirming {
		t.Fatal("expected prune confirmation")
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
	m = updated.(Model)
	if !m.PruneRunning {
		t.Fatal("expected prune operation to start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.PruneRunning {
		t.Fatal("expected prune operation to finish")
	}
	if m.Status.Kind() != "success" {
		t.Fatalf("status kind = %q, want success", m.Status.Kind())
	}
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
