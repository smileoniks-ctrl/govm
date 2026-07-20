package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// TestInstalledTab_UKeyTriggersSwitchVersion regression-tests a bug
// where pressing `u` on the Installed tab was a no-op: handleUseKey
// only had branches for AvailableTab and DepsTab, so the key fell
// through and returned a nil command. The Installed tab must read its
// own selected row (InstalledTable.SelectedRow) and dispatch
// SwitchVersion for the matching version.
func TestInstalledTab_UKeyTriggersSwitchVersion(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	m.Versions = []utils.GoVersion{
		{Version: "1.24.4", Installed: true, Active: true, Path: "/p/1.24.4"},
		{Version: "1.26.0", Installed: true, Active: false, Path: "/p/1.26.0"},
	}
	m.rebuildVersionViews()
	m.InstalledTable.Focus()
	// Move the cursor to the non-active row (1.26.0).
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	if got := m.InstalledTable.Cursor(); got != 1 {
		t.Fatalf("cursor = %d, want 1 before pressing u", got)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'u'})
	got := updated.(Model)

	if cmd == nil {
		t.Fatal("expected SwitchVersion command when pressing 'u' on Installed tab, got nil")
	}
	if !got.Loading {
		t.Fatal("expected Loading=true while switch is in flight")
	}
}

// TestDepsTab_CheckStatusClearsAfterDependenciesMsg regression-tests
// a bug where "Checking for dependency updates..." leaked across all
// tabs after pressing `r` on the Deps tab. handleRefreshKey used
// setGlobalStatus (global scope) but the DependenciesMsg handler
// called clearTabStatus (tab scope only), so the message was never
// torn down and survived tab switches.
func TestDepsTab_CheckStatusClearsAfterDependenciesMsg(t *testing.T) {
	m := newTestModel(t)
	// Switch to DepsTab (also lazy-loads).
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(utils.DependenciesMsg{})
	m = updated.(Model)

	// Press 'r' to start a check.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)
	if m.Deps.Phase != OpChecking {
		t.Fatalf("phase = %v, want OpChecking", m.Deps.Phase)
	}

	// Simulate the check completing.
	updated, _ = m.Update(utils.DependenciesMsg{})
	m = updated.(Model)

	if m.Message != "" {
		t.Fatalf("expected status to be cleared after DependenciesMsg, got %q", m.Message)
	}
	if m.Deps.Phase != OpIdle {
		t.Fatalf("phase = %v, want OpIdle after DependenciesMsg", m.Deps.Phase)
	}
}

// TestDepsTab_CheckPhaseShowsSpinner regression-tests a bug where the
// Deps tab `r` (check updates) flow showed a static status line with
// no spinner animation, unlike the Available tab refresh. The root
// cause was that SpinnerText returned "" for OpChecking/OpUpdating
// while an imperative setGlobalStatus filled m.Message with a non-
// empty string; composeStatus therefore fell through its spinner
// branches and never prefixed Spinner.View().
func TestDepsTab_CheckPhaseShowsSpinner(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(utils.DependenciesMsg{})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)

	status, _ := m.composeStatus()
	plain := stripANSI(status)

	if !strings.Contains(plain, stripANSI(m.Spinner.View())) {
		t.Fatalf("expected status to contain spinner frame %q, got %q", stripANSI(m.Spinner.View()), plain)
	}
	if !strings.Contains(plain, "Checking") {
		t.Fatalf("expected status to mention 'Checking', got %q", plain)
	}
}
