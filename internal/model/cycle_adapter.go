package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

type dependencyExecutionErrMsg struct {
	Err error
}

// handleCycleEvent feeds a deps.Event (received as a tea.Msg from an
// operational tea.Cmd) into the Cycle and dispatches the resulting
// intent. Operational intents become a tea.Cmd that runs the executor;
// confirmation intents open the matching dialog; a terminal outcome
// (NoIntent) is rendered as a status message.
func (m *Model) handleCycleEvent(event deps.Event) (tea.Model, tea.Cmd) {
	next, intent, err := m.Deps.Cycle.Handle(event)
	if err != nil {
		m.Deps.Cycle = deps.NewUpdateCycle()
		m.Status.SetGlobal(err.Error(), "error")
		return m, nil
	}
	m.Deps.Cycle = next
	return m.applyCycleIntent(intent)
}

// applyCycleIntent maps a Cycle intent to its TUI side effect.
func (m *Model) applyCycleIntent(intent deps.Intent) (tea.Model, tea.Cmd) {
	switch i := intent.(type) {
	case deps.NoIntent:
		return m.applyCycleTerminal()
	case deps.IntentCheckUpdates, deps.IntentApplyUpdates,
		deps.IntentCompensate, deps.IntentRunChecks, deps.IntentRollback:
		return m.applyCycleOperational(i)
	case deps.IntentConfirmApply:
		choice := i.DefaultYes
		// The explicit set (marks, else the cursor module) is frozen
		// for the whole cycle: keys that change marks are inert while
		// the cycle runs, so reading it here is stable.
		explicitModules := m.Deps.explicitModules()
		if m.Deps.Dialog.Kind == DialogUpdate {
			// A level or scope change re-emits the intent: keep the
			// button the user had highlighted and the captured set.
			choice = m.Deps.Dialog.ChoiceYes
			explicitModules = m.Deps.Dialog.ExplicitModules
		}
		m.Deps.Dialog = ConfirmDialog{
			Kind:            DialogUpdate,
			ChoiceYes:       choice,
			UpdateEntries:   i.Entries,
			Level:           i.Level,
			Explicit:        i.Explicit,
			ExplicitModules: explicitModules,
		}
		m.Status.Clear()
		return m, nil
	case deps.IntentConfirmChecks:
		m.syncDepsFromCycle()
		n := i.UpdatedCount
		m.Status.SetGlobal(fmt.Sprintf(
			"Updated %d direct %s. Run checks?",
			n, deps.Pluralize(n, "dependency", "dependencies")), "success")
		m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: i.DefaultYes}
		return m, nil
	case deps.IntentConfirmRollback:
		m.Deps.Dialog = ConfirmDialog{
			Kind:         DialogRollback,
			ChoiceYes:    i.DefaultYes,
			Inconclusive: i.CheckErr != nil,
			CheckResult:  i.CheckResult,
		}
		if i.CheckErr != nil {
			m.Status.SetGlobal(fmt.Sprintf("Checks could not run: %s", i.CheckErr), "error")
		} else if i.CheckResult != nil {
			m.Status.SetGlobal(fmt.Sprintf("Checks failed: %s", i.CheckResult.Command), "error")
		}
		return m, nil
	}
	m.Deps.Cycle = deps.NewUpdateCycle()
	m.Status.SetGlobal(fmt.Sprintf("Unhandled dependency cycle intent %T", intent), "error")
	return m, nil
}

// applyCycleOperational sets the progress status and returns the tea.Cmd
// that runs the operational intent through the execution seam.
func (m *Model) applyCycleOperational(intent deps.Intent) (tea.Model, tea.Cmd) {
	switch intent.(type) {
	case deps.IntentApplyUpdates:
		m.Status.SetGlobal("Updating dependencies...", "info")
	case deps.IntentCompensate:
		m.Status.SetGlobal("Reverting partial update...", "info")
	case deps.IntentRunChecks:
		m.Status.SetGlobal("Running checks...", "info")
	case deps.IntentRollback:
		m.Status.SetGlobal("Rolling back dependencies...", "info")
	}
	return m, m.cycleExecuteCmd(intent)
}

// applyCycleTerminal renders the terminal outcome as a status message,
// syncs the dependency table, and resets the Cycle to idle.
func (m *Model) applyCycleTerminal() (tea.Model, tea.Cmd) {
	c := m.Deps.Cycle
	// Marks describe the next update; once a cycle has run its course
	// they are spent. Cancelling in the apply dialog keeps them so the
	// user can adjust and retry.
	if c.Outcome() != deps.OutcomeApplyCanceled {
		m.Deps.ClearMarks()
		m.updateDependencyTable()
	}
	if c.Outcome() != deps.OutcomeRecoveryRequired && c.Outcome() != deps.OutcomeFailed {
		m.syncDepsFromCycle()
	}
	switch c.Outcome() {
	case deps.OutcomeNoUpdates:
		m.Status.SetTab(noUpdatesMessage(c.Selection()), "warning")
	case deps.OutcomeApplyCanceled:
		m.Status.SetTab("Update canceled.", "info")
	case deps.OutcomeUpdatedUnchecked:
		m.Status.SetGlobal("Update complete. Checks skipped.", "info")
	case deps.OutcomeUpdatedVerified:
		m.Status.SetGlobal("Checks passed.", "success")
	case deps.OutcomeUpdatesKeptWithFailedChecks:
		m.Status.SetGlobal("Update kept. Failed checks were not rolled back.", "warning")
	case deps.OutcomeRolledBack:
		m.Status.SetGlobal("Rolled back to pre-update state.", "success")
	case deps.OutcomeUpdateFailedRestored:
		m.Status.SetGlobal(cycleUpdateFailedRestoredMessage(c), "warning")
	case deps.OutcomeRecoveryRequired:
		m.Status.SetGlobal(cycleRecoveryRequiredMessage(c), "error")
	case deps.OutcomeFailed:
		if c.Failure() != nil {
			m.Status.SetGlobal(c.Failure().Error(), "error")
		} else {
			m.Status.SetGlobal("Update failed.", "error")
		}
	}
	m.Deps.Cycle = deps.NewUpdateCycle()
	return m, nil
}

// noUpdatesMessage renders the no-updates outcome: bulk selections
// report on direct dependencies, explicit ones name the modules.
func noUpdatesMessage(sel deps.UpdateSelection) string {
	suffix := ""
	if sel.Level != deps.LevelLatest {
		suffix = fmt.Sprintf(" at the %s level", sel.Level)
	}
	if !sel.Explicit() {
		return fmt.Sprintf("No direct dependency updates available%s.", suffix)
	}
	const maxNamed = 3
	names := sel.Modules
	extra := ""
	if len(names) > maxNamed {
		extra = fmt.Sprintf(" …and %d more", len(names)-maxNamed)
		names = names[:maxNamed]
	}
	return fmt.Sprintf("Already up to date%s: %s%s", suffix, strings.Join(names, ", "), extra)
}

func cycleUpdateFailedRestoredMessage(c deps.UpdateCycle) string {
	if c.Failure() != nil {
		return fmt.Sprintf("Update failed and was reverted: %s", c.Failure())
	}
	return "Update failed and was reverted."
}

func cycleRecoveryRequiredMessage(c deps.UpdateCycle) string {
	msg := "Recovery required."
	if c.Failure() != nil {
		msg = fmt.Sprintf("Recovery required: %s", c.Failure())
	}
	if b := c.Backup(); b != nil && b.Name != "" {
		if b.Path != "" {
			msg = fmt.Sprintf("%s A backup was saved: %s (%s)", msg, b.Name, b.Path)
		} else {
			msg = fmt.Sprintf("%s A backup was saved: %s", msg, b.Name)
		}
	}
	return msg
}

// depsExecutor returns the dependency executor bound to the current
// Settings backup limit, or the unavailable executor while none is
// bound.
func (m Model) depsExecutor() DepsExecutor {
	if m.Deps.Executor == nil {
		return unavailableDepsExecutor{}
	}
	return m.Deps.Executor(m.Settings.Values.DepsBackupLimit)
}

// cycleExecuteCmd builds the tea.Cmd that runs an operational intent
// through the dependency executor.
func (m Model) cycleExecuteCmd(intent deps.Intent) tea.Cmd {
	executor := m.depsExecutor()
	return func() tea.Msg {
		event, err := executor.Execute(intent)
		if err != nil {
			return dependencyExecutionErrMsg{Err: err}
		}
		return event
	}
}

// syncDepsFromCycle copies the Cycle's current dependency list into the
// presentation table when the Cycle has one (post-check, post-apply,
// post-rollback). It is a no-op when the Cycle has no dependencies.
func (m *Model) syncDepsFromCycle() {
	d := m.Deps.Cycle.Dependencies()
	if d != nil {
		m.Deps.Dependencies = d
		m.updateDependencyTable()
	}
}
