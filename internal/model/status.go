package model

// clearDeleteContext resets the delete-confirmation context on Model.
// It is the half of the historical clearTabContext that does not touch
// the status line; clearTabContext composes this with Status.ClearTab
// so the tab-switch path still tears down both in one call.
func (m *Model) clearDeleteContext() {
	m.ConfirmingDelete = false
	m.DeleteVersion = ""
}

// clearTabContext tears down everything the current tab accumulated:
// the tab-scoped status line and the delete-confirmation context.
// Global-scoped status messages survive (a successful install should
// still be visible after switching tabs).
func (m *Model) clearTabContext() {
	m.Status.ClearTab()
	m.clearDeleteContext()
	m.Prune.Reset()
}
