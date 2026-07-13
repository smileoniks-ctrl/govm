package model

type statusScope uint8

const (
	statusScopeTab statusScope = iota
	statusScopeGlobal
)

func (m *Model) setTabStatus(message, messageType string) {
	m.Message = message
	m.MessageType = messageType
	m.MessageScope = statusScopeTab
}

func (m *Model) setGlobalStatus(message, messageType string) {
	m.Message = message
	m.MessageType = messageType
	m.MessageScope = statusScopeGlobal
}

func (m *Model) clearStatus() {
	m.setTabStatus("", "")
}

func (m *Model) clearTabStatus() {
	if m.MessageScope == statusScopeTab {
		m.clearStatus()
	}
}

func (m *Model) clearTabContext() {
	m.clearTabStatus()
	m.ConfirmingDelete = false
	m.DeleteVersion = ""
}
