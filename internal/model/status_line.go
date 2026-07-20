package model

// StatusLine is the value-type module that owns the status triplet:
// text, kind, and scope. It is the fifth application of the "implicit
// module in flat fields -> explicit value-type module" pattern in this
// package (after ConfirmDialog, DepsOperation, rebuildVersionViews,
// and Theme).
//
// The zero value is an empty, tab-scoped, inactive status. Reads go
// through Text/Kind/Scope; mutations go through SetTab/SetGlobal/
// Clear/ClearTab. The legacy Model.Message/MessageType/MessageScope
// fields and their four helpers in status.go are removed once every
// call site migrates to StatusLine.
//
// Receiver style mirrors DepsState/SettingsState: the value lives as a
// field on Model, reads use value receivers, mutations use pointer
// receivers invoked from *Model methods (m.Status.SetGlobal(...)).
type StatusLine struct {
	text  string
	kind  string
	scope statusScope
}

// Text returns the current status message text ("" when inactive).
func (s StatusLine) Text() string { return s.text }

// Kind returns the status type ("success", "error", "warning", "info").
func (s StatusLine) Kind() string { return s.kind }

// Scope returns the scope the current message applies to.
func (s StatusLine) Scope() statusScope { return s.scope }

// SetTab records a tab-scoped status message. The message is torn down
// by ClearTab when the user leaves the tab.
func (s *StatusLine) SetTab(message, kind string) {
	s.text = message
	s.kind = kind
	s.scope = statusScopeTab
}

// SetGlobal records a global-scoped status message that survives tab
// switches (e.g. "Successfully installed Go 1.24.4").
func (s *StatusLine) SetGlobal(message, kind string) {
	s.text = message
	s.kind = kind
	s.scope = statusScopeGlobal
}

// Clear resets the status to empty. It deliberately sets scope = Tab,
// not "preserve previous scope with empty text": ClearTab only acts
// when scope == Tab, so Clear must leave the status in a state where a
// subsequent ClearTab can still run. Clearing while leaving scope ==
// Global would make ClearTab a no-op and leak a global status that can
// no longer be torn down by the tab-switch path.
func (s *StatusLine) Clear() {
	s.text = ""
	s.kind = ""
	s.scope = statusScopeTab
}

// ClearTab clears the status only when it is tab-scoped; a global
// message (e.g. a just-completed install) is preserved across tab
// switches.
func (s *StatusLine) ClearTab() {
	if s.scope == statusScopeTab {
		s.Clear()
	}
}
