package model

import (
	"fmt"

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
func (m Model) handleCycleEvent(event deps.Event) (tea.Model, tea.Cmd) {
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
func (m Model) applyCycleIntent(intent deps.Intent) (tea.Model, tea.Cmd) {
	switch i := intent.(type) {
	case deps.NoIntent:
		return m.applyCycleTerminal()
	case deps.IntentCheckUpdates, deps.IntentApplyUpdates,
		deps.IntentCompensate, deps.IntentRunChecks, deps.IntentRollback:
		return m.applyCycleOperational(i)
	case deps.IntentConfirmApply:
		m.Deps.Dialog = ConfirmDialog{
			Kind:          DialogUpdate,
			ChoiceYes:     i.DefaultYes,
			UpdateEntries: i.Entries,
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
func (m Model) applyCycleOperational(intent deps.Intent) (tea.Model, tea.Cmd) {
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
func (m Model) applyCycleTerminal() (tea.Model, tea.Cmd) {
	c := m.Deps.Cycle
	if c.Outcome() != deps.OutcomeRecoveryRequired && c.Outcome() != deps.OutcomeFailed {
		m.syncDepsFromCycle()
	}
	switch c.Outcome() {
	case deps.OutcomeNoUpdates:
		m.Status.SetTab("No direct dependency updates available.", "warning")
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

// cycleExecuteCmd builds the tea.Cmd that runs an operational intent.
// When the ExecuteIntent seam is set (tests) it is used directly;
// otherwise a real deps.Executor is built from the current Settings
// backup limit so a mid-session limit change is always respected.
func (m Model) cycleExecuteCmd(intent deps.Intent) tea.Cmd {
	if fn := m.Deps.ExecuteIntent; fn != nil {
		return fn(intent)
	}
	moduleDir := m.Deps.ModuleDir
	limit := m.Settings.Values.DepsBackupLimit
	return func() tea.Msg {
		exec, err := deps.NewExecutor(moduleDir, nil, limit)
		if err != nil {
			return dependencyExecutionErrMsg{Err: err}
		}
		event, err := exec.Execute(intent)
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
