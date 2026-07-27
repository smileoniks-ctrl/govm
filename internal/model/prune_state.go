package model

import "github.com/smileoniks-ctrl/govm/internal/prune"

// prunePhase is the phase of the prune flow. The zero value is idle, so a
// zero PruneState is a valid idle state and needs no constructor.
type prunePhase int

const (
	prunePhaseIdle prunePhase = iota
	prunePhasePreviewing
	prunePhaseConfirming
	prunePhaseRunning
)

// PruneState owns the prune flow: its phase and the plan awaiting
// confirmation. It is the single place that decides whether a prune may
// start, so callers ask it instead of re-deriving the rule.
//
// Reads use value receivers; transitions use pointer receivers and report
// whether they were allowed, so a caller can bail out without touching the
// status line or emitting a command.
type PruneState struct {
	phase prunePhase
	plan  prune.Result
}

// Busy reports whether a prune flow is in progress in any phase.
func (s PruneState) Busy() bool { return s.phase != prunePhaseIdle }

// Confirming reports whether the plan is waiting for the user's answer.
func (s PruneState) Confirming() bool { return s.phase == prunePhaseConfirming }

// Plan returns the plan awaiting confirmation. It is empty outside the
// confirming phase.
func (s PruneState) Plan() prune.Result { return s.plan }

// BeginPreview moves idle -> previewing.
func (s *PruneState) BeginPreview() bool {
	if s.phase != prunePhaseIdle {
		return false
	}
	s.phase = prunePhasePreviewing
	return true
}

// AcceptPreview records the preview outcome: previewing -> confirming when
// the plan has candidates, previewing -> idle when it has none. It reports
// whether there is something to confirm. A preview error is not its
// business: it changes neither the phase nor the plan, only the text the
// caller puts on the status line.
func (s *PruneState) AcceptPreview(res prune.Result) bool {
	if s.phase != prunePhasePreviewing {
		return false
	}
	if len(res.Candidates) == 0 {
		s.phase = prunePhaseIdle
		s.plan = prune.Result{}
		return false
	}
	s.phase = prunePhaseConfirming
	s.plan = res
	return true
}

// Confirm moves confirming -> running.
func (s *PruneState) Confirm() bool {
	if s.phase != prunePhaseConfirming {
		return false
	}
	s.phase = prunePhaseRunning
	return true
}

// Cancel moves confirming -> idle after the user declines the plan. It is
// kept apart from Finish because the preconditions differ: merging them
// would cost the machine its ability to tell an allowed transition from a
// disallowed one.
func (s *PruneState) Cancel() bool {
	if s.phase != prunePhaseConfirming {
		return false
	}
	s.reset()
	return true
}

// Finish moves running -> idle once the operation reports back.
func (s *PruneState) Finish() { s.reset() }

// Reset is the total teardown used when the user leaves the tab; it acts
// from any phase.
func (s *PruneState) Reset() { s.reset() }

func (s *PruneState) reset() {
	s.phase = prunePhaseIdle
	s.plan = prune.Result{}
}
