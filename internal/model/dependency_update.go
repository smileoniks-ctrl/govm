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
		m.UpdateChoiceYes = !m.UpdateChoiceYes
		return m, nil
	case "enter":
		return m.applyUpdateChoice()
	case "y", "Y":
		m.UpdateChoiceYes = true
		return m.applyUpdateChoice()
	case "n", "N", "esc":
		m.ConfirmingDependencyUpdate = false
		m.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}
	return m, nil
}

func (m Model) applyUpdateChoice() (tea.Model, tea.Cmd) {
	if !m.UpdateChoiceYes {
		m.ConfirmingDependencyUpdate = false
		m.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}

	m.ConfirmingDependencyUpdate = false
	m.UpdateChoiceYes = false
	m.UpdatingDependencies = true
	m.Message = "Updating dependencies..."
	m.MessageType = "info"
	return m, utils.UpdateModuleDependencies(m.ModuleDir, m.Dependencies)
}

// handleChecksConfirmKey handles key presses while the "run checks?"
// dialog is open. The default choice is Yes to encourage users to
// verify their upgrade.
func (m Model) handleChecksConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.CheckChoiceYes = !m.CheckChoiceYes
		return m, nil
	case "enter":
		return m.applyChecksChoice()
	case "y", "Y":
		m.CheckChoiceYes = true
		return m.applyChecksChoice()
	case "n", "N", "esc":
		m.ConfirmingDependencyChecks = false
		m.CheckChoiceYes = false
		m.Message = "Update complete. Checks skipped."
		m.MessageType = "info"
		return m, nil
	}
	return m, nil
}

func (m Model) applyChecksChoice() (tea.Model, tea.Cmd) {
	if !m.CheckChoiceYes {
		m.ConfirmingDependencyChecks = false
		m.CheckChoiceYes = false
		m.Message = "Update complete. Checks skipped."
		m.MessageType = "info"
		return m, nil
	}

	m.ConfirmingDependencyChecks = false
	m.CheckChoiceYes = false
	m.RunningDependencyChecks = true
	m.Message = "Running checks..."
	m.MessageType = "info"
	return m, utils.RunModuleDependencyChecks(m.ModuleDir)
}

// handleRollbackConfirmKey handles key presses while the rollback
// confirmation dialog is open. The default choice is Rollback to
// make the safe option the easiest one.
func (m Model) handleRollbackConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.RollbackChoiceYes = !m.RollbackChoiceYes
		return m, nil
	case "enter":
		return m.applyRollbackChoice()
	case "y", "Y":
		m.RollbackChoiceYes = true
		return m.applyRollbackChoice()
	case "n", "N", "esc":
		return m.keepUpdatedDependencies()
	}
	return m, nil
}

func (m Model) applyRollbackChoice() (tea.Model, tea.Cmd) {
	if !m.RollbackChoiceYes {
		return m.keepUpdatedDependencies()
	}

	if m.LastDependencySnapshot == nil {
		m.ConfirmingDependencyRollback = false
		m.RollbackChoiceYes = false
		m.Message = "Rollback unavailable: snapshot is missing."
		m.MessageType = "error"
		return m, nil
	}

	m.ConfirmingDependencyRollback = false
	m.RollbackChoiceYes = false
	m.RollingBackDependencies = true
	m.Message = "Rolling back dependencies..."
	m.MessageType = "info"
	snap := m.LastDependencySnapshot
	return m, utils.RollbackModuleDependencies(m.ModuleDir, snap)
}

func (m Model) keepUpdatedDependencies() (tea.Model, tea.Cmd) {
	m.ConfirmingDependencyRollback = false
	m.RollbackChoiceYes = false
	m.Message = "Update kept. Failed checks were not rolled back."
	m.MessageType = "warning"
	return m, nil
}
