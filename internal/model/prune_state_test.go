package model

import (
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/prune"
)

func prunePlanWithCandidates() prune.Result {
	return prune.Result{
		Candidates: []prune.Candidate{{
			Path:    "/versions/go1.23.0",
			Version: "1.23.0",
			Bytes:   1024,
			Kind:    prune.CandidateVersion,
		}},
	}
}

func stateInPhase(t *testing.T, phase prunePhase) PruneState {
	t.Helper()
	var s PruneState
	switch phase {
	case prunePhaseIdle:
	case prunePhasePreviewing:
		s.BeginPreview()
	case prunePhaseConfirming:
		s.BeginPreview()
		s.AcceptPreview(prunePlanWithCandidates())
	case prunePhaseRunning:
		s.BeginPreview()
		s.AcceptPreview(prunePlanWithCandidates())
		s.Confirm()
	default:
		t.Fatalf("unknown phase %d", phase)
	}
	if s.phase != phase {
		t.Fatalf("setup reached phase %d, want %d", s.phase, phase)
	}
	return s
}

var allPrunePhases = []prunePhase{
	prunePhaseIdle,
	prunePhasePreviewing,
	prunePhaseConfirming,
	prunePhaseRunning,
}

func TestPruneStateAllowedTransitions(t *testing.T) {
	plan := prunePlanWithCandidates()

	var s PruneState
	if s.Busy() {
		t.Fatal("zero PruneState must be idle")
	}
	if !s.BeginPreview() {
		t.Fatal("BeginPreview from idle must be allowed")
	}
	if !s.Busy() || s.Confirming() {
		t.Fatal("previewing must be busy but not confirming")
	}
	if !s.AcceptPreview(plan) {
		t.Fatal("AcceptPreview with candidates must move to confirming")
	}
	if !s.Confirming() {
		t.Fatal("expected confirming phase")
	}
	if len(s.Plan().Candidates) != 1 {
		t.Fatalf("plan candidates = %d, want 1", len(s.Plan().Candidates))
	}
	if !s.Confirm() {
		t.Fatal("Confirm from confirming must be allowed")
	}
	if s.phase != prunePhaseRunning {
		t.Fatalf("phase = %d, want running", s.phase)
	}
	s.Finish()
	if s.Busy() {
		t.Fatal("Finish must return to idle")
	}
	if len(s.Plan().Candidates) != 0 {
		t.Fatal("Finish must clear the plan")
	}
}

func TestPruneStateCancelFromConfirming(t *testing.T) {
	s := stateInPhase(t, prunePhaseConfirming)
	if !s.Cancel() {
		t.Fatal("Cancel from confirming must be allowed")
	}
	if s.Busy() {
		t.Fatal("Cancel must return to idle")
	}
	if len(s.Plan().Candidates) != 0 {
		t.Fatal("Cancel must clear the plan")
	}
}

func TestPruneStateAcceptPreviewWithoutCandidates(t *testing.T) {
	s := stateInPhase(t, prunePhasePreviewing)
	if s.AcceptPreview(prune.Result{}) {
		t.Fatal("AcceptPreview without candidates must report nothing to confirm")
	}
	if s.Busy() {
		t.Fatal("AcceptPreview without candidates must return to idle")
	}
	if len(s.Plan().Candidates) != 0 {
		t.Fatal("plan must stay empty")
	}
}

func TestPruneStateRejectedTransitions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		from  []prunePhase
		apply func(*PruneState) bool
	}{
		{
			name:  "BeginPreview outside idle",
			from:  []prunePhase{prunePhasePreviewing, prunePhaseConfirming, prunePhaseRunning},
			apply: (*PruneState).BeginPreview,
		},
		{
			name:  "Confirm outside confirming",
			from:  []prunePhase{prunePhaseIdle, prunePhasePreviewing, prunePhaseRunning},
			apply: (*PruneState).Confirm,
		},
		{
			name:  "Cancel outside confirming",
			from:  []prunePhase{prunePhaseIdle, prunePhasePreviewing, prunePhaseRunning},
			apply: (*PruneState).Cancel,
		},
		{
			name: "AcceptPreview outside previewing",
			from: []prunePhase{prunePhaseIdle, prunePhaseConfirming, prunePhaseRunning},
			apply: func(s *PruneState) bool {
				return s.AcceptPreview(prunePlanWithCandidates())
			},
		},
	} {
		for _, phase := range tc.from {
			s := stateInPhase(t, phase)
			before := s
			if tc.apply(&s) {
				t.Errorf("%s: transition from phase %d must be rejected", tc.name, phase)
			}
			if s.phase != before.phase {
				t.Errorf("%s: phase changed from %d to %d", tc.name, before.phase, s.phase)
			}
			if len(s.Plan().Candidates) != len(before.Plan().Candidates) {
				t.Errorf("%s: plan changed in phase %d", tc.name, phase)
			}
		}
	}
}

func TestPruneStateTeardownFromEveryPhase(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*PruneState)
	}{
		{"Finish", (*PruneState).Finish},
		{"Reset", (*PruneState).Reset},
	} {
		for _, phase := range allPrunePhases {
			s := stateInPhase(t, phase)
			tc.apply(&s)
			if s.Busy() {
				t.Errorf("%s from phase %d left the state busy", tc.name, phase)
			}
			if len(s.Plan().Candidates) != 0 {
				t.Errorf("%s from phase %d left a plan behind", tc.name, phase)
			}
		}
	}
}
