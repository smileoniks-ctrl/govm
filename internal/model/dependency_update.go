package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// handleUpdateConfirmKey handles key presses while the dependency
// update confirmation dialog is open.
func (m Model) handleUpdateConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.Deps.Dialog.UpdateChoiceYes = !m.Deps.Dialog.UpdateChoiceYes
		return m, nil
	case "enter":
		return m.applyUpdateChoice()
	case "y", "Y":
		m.Deps.Dialog.UpdateChoiceYes = true
		return m.applyUpdateChoice()
	case "n", "N", "esc":
		m.Deps.Dialog.ConfirmingUpdate = false
		m.Deps.Dialog.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}
	return m, nil
}

func (m Model) applyUpdateChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.UpdateChoiceYes {
		m.Deps.Dialog.ConfirmingUpdate = false
		m.Deps.Dialog.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}

	m.Deps.Dialog.ConfirmingUpdate = false
	m.Deps.Dialog.UpdateChoiceYes = false
	m.Deps.Updating = true
	m.Message = "Updating dependencies..."
	m.MessageType = "info"
	return m, utils.UpdateModuleDependencies(m.Deps.ModuleDir, m.Deps.Dependencies)
}

// handleChecksConfirmKey handles key presses while the "run checks?"
// dialog is open. The default choice is Yes to encourage users to
// verify their upgrade.
func (m Model) handleChecksConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.Deps.Dialog.CheckChoiceYes = !m.Deps.Dialog.CheckChoiceYes
		return m, nil
	case "enter":
		return m.applyChecksChoice()
	case "y", "Y":
		m.Deps.Dialog.CheckChoiceYes = true
		return m.applyChecksChoice()
	case "n", "N", "esc":
		m.Deps.Dialog.ConfirmingChecks = false
		m.Deps.Dialog.CheckChoiceYes = false
		m.Message = "Update complete. Checks skipped."
		m.MessageType = "info"
		return m, nil
	}
	return m, nil
}

func (m Model) applyChecksChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.CheckChoiceYes {
		m.Deps.Dialog.ConfirmingChecks = false
		m.Deps.Dialog.CheckChoiceYes = false
		m.Message = "Update complete. Checks skipped."
		m.MessageType = "info"
		return m, nil
	}

	m.Deps.Dialog.ConfirmingChecks = false
	m.Deps.Dialog.CheckChoiceYes = false
	m.Deps.RunningChecks = true
	m.Message = "Running checks..."
	m.MessageType = "info"
	return m, utils.RunModuleDependencyChecks(m.Deps.ModuleDir)
}

// handleRollbackConfirmKey handles key presses while the rollback
// confirmation dialog is open. The default choice is Rollback to
// make the safe option the easiest one.
func (m Model) handleRollbackConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.Deps.Dialog.RollbackChoiceYes = !m.Deps.Dialog.RollbackChoiceYes
		return m, nil
	case "enter":
		return m.applyRollbackChoice()
	case "y", "Y":
		m.Deps.Dialog.RollbackChoiceYes = true
		return m.applyRollbackChoice()
	case "n", "N", "esc":
		return m.keepUpdatedDependencies()
	}
	return m, nil
}

func (m Model) applyRollbackChoice() (tea.Model, tea.Cmd) {
	if !m.Deps.Dialog.RollbackChoiceYes {
		return m.keepUpdatedDependencies()
	}

	if m.Deps.Snapshot == nil {
		m.Deps.Dialog.ConfirmingRollback = false
		m.Deps.Dialog.RollbackChoiceYes = false
		m.Message = "Rollback unavailable: snapshot is missing."
		m.MessageType = "error"
		return m, nil
	}

	m.Deps.Dialog.ConfirmingRollback = false
	m.Deps.Dialog.RollbackChoiceYes = false
	m.Deps.RollingBack = true
	m.Message = "Rolling back dependencies..."
	m.MessageType = "info"
	snap := m.Deps.Snapshot
	return m, utils.RollbackModuleDependencies(m.Deps.ModuleDir, snap)
}

func (m Model) keepUpdatedDependencies() (tea.Model, tea.Cmd) {
	m.Deps.Dialog.ConfirmingRollback = false
	m.Deps.Dialog.RollbackChoiceYes = false
	m.Message = "Update kept. Failed checks were not rolled back."
	m.MessageType = "warning"
	return m, nil
}
