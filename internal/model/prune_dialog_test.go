package model

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/prune"
)

func pruneConfirmingModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	m.previewPrune = func(context.Context) (prune.Result, error) {
		return prune.Result{
			Candidates: []prune.Candidate{{
				Path:    "/versions/go1.26.1",
				Version: "1.26.1",
				Bytes:   1024,
				Kind:    prune.CandidateVersion,
			}},
		}, nil
	}
	m.runPrune = func(context.Context) (prune.Result, error) {
		return prune.Result{}, nil
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if !m.Prune.Confirming() {
		t.Fatal("expected prune confirmation")
	}
	return m
}

func TestPruneDialogRendersWarningTitleAndButtons(t *testing.T) {
	m := pruneConfirmingModel(t)
	got := stripANSI(m.View().Content)
	for _, want := range []string{"⚠", "Prune inactive Go versions?", "Reclaimable: 1.0 KiB", "1.26.1", "Yes", "No"} {
		if !strings.Contains(got, want) {
			t.Errorf("prune dialog missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Press Y to confirm") {
		t.Errorf("prune dialog still shows the y/n-only hint:\n%s", got)
	}
}

func TestPruneDialogArrowsAndEnterCancel(t *testing.T) {
	m := pruneConfirmingModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.Prune.Busy() {
		t.Fatalf("expected ←+enter to cancel the prune, phase = %d", m.Prune.phase)
	}
}

func TestPruneDialogEnterConfirmsDefaultYes(t *testing.T) {
	m := pruneConfirmingModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.Prune.phase != prunePhaseRunning {
		t.Fatalf("expected enter to confirm the prune, phase = %d", m.Prune.phase)
	}
}

func TestPruneDialogEscCancels(t *testing.T) {
	m := pruneConfirmingModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.Prune.Busy() {
		t.Fatalf("expected esc to cancel the prune, phase = %d", m.Prune.phase)
	}
}
