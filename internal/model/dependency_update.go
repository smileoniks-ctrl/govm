package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) handleUpdateConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.UpdateChoiceYes = !m.UpdateChoiceYes
		return m, nil
	case "enter":
		return m.applyUpdateChoice()
	case "y", "Y", "д", "Д":
		m.UpdateChoiceYes = true
		return m.applyUpdateChoice()
	case "n", "N", "н", "Н", "esc":
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
