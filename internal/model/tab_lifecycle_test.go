package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestTabSwitchClearsTabLocalStatus(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Status.SetTab("No direct dependency updates available.", "warning")

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	got := updated.(Model)

	if got.CurrentTab != SettingsTab {
		t.Fatalf("current tab = %d, want %d", got.CurrentTab, SettingsTab)
	}
	if got.Status.Text() != "" || got.Status.Kind() != "" {
		t.Fatalf("tab-local status = (%q, %q), want empty", got.Status.Text(), got.Status.Kind())
	}
}

func TestTabSwitchPreservesGlobalStatus(t *testing.T) {
	m := newTestModel(t)
	m.Status.SetGlobal("Successfully installed Go 1.24.4", "success")

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	got := updated.(Model)

	if got.CurrentTab != InstalledTab {
		t.Fatalf("current tab = %d, want %d", got.CurrentTab, InstalledTab)
	}
	if got.Status.Text() != "Successfully installed Go 1.24.4" {
		t.Fatalf("message = %q, want preserved global status", got.Status.Text())
	}
	if got.Status.Kind() != "success" {
		t.Fatalf("message type = %q, want %q", got.Status.Kind(), "success")
	}
}

// TestTabSwitchClearsSwitchedToGoStatus regression-tests the UX rule
// that the "Switched to Go X" success message is scoped to the tab it
// was produced on: once the user moves to another tab the message must
// disappear. The SwitchCompletedMsg handler previously used
// Status.SetGlobal, so the message survived tab switches and felt like
// a stale warning stuck on screen.
func TestTabSwitchClearsSwitchedToGoStatus(t *testing.T) {
	m := newTestModel(t)
	// Seed the switch target as an installed (but inactive) version so
	// the SwitchCompletedMsg handler can activate it directly through
	// the catalog without entering reconciliation.
	seedVersions(t, &m, []utils.GoVersion{
		{Version: "1.24.4", Filename: "go1.24.4.darwin-arm64.tar.gz", Installed: true, Active: true, Path: "/p/1.24.4"},
		{Version: "1.26.5", Filename: "go1.26.5.darwin-arm64.tar.gz", Installed: true, Active: false, Path: "/p/1.26.5"},
	})
	// Simulate the switch completing with the shim already on PATH,
	// which is the branch that produces "Switched to Go X! Run ...".
	updated, _ := m.Update(activationSuccessMsg{
		Result:     lifecycle.ActivationResult{Version: "1.26.5"},
		ShimInPath: true,
	})
	m = updated.(Model)

	if got, want := m.Status.Text(), "Switched to Go 1.26.5! Run 'go version' to verify."; got != want {
		t.Fatalf("status after switch = %q, want %q", got, want)
	}

	// Switching tabs must tear down the tab-scoped success message.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	got := updated.(Model)

	if got.Status.Text() != "" || got.Status.Kind() != "" {
		t.Fatalf("status after tab switch = (%q, %q), want empty", got.Status.Text(), got.Status.Kind())
	}
}

func TestTabSwitchCancelsPendingDelete(t *testing.T) {
	m := newTestModel(t)
	m.Status.SetTab("Are you sure you want to delete Go 1.24.4?", "warning")
	m.ConfirmingDelete = true
	m.DeleteVersion = "1.24.4"

	updated, _ := m.handleTabKey()
	got := updated.(Model)

	if got.CurrentTab != InstalledTab {
		t.Fatalf("current tab = %d, want %d", got.CurrentTab, InstalledTab)
	}
	if got.ConfirmingDelete {
		t.Fatal("ConfirmingDelete = true, want false")
	}
	if got.DeleteVersion != "" {
		t.Fatalf("DeleteVersion = %q, want empty", got.DeleteVersion)
	}
	if got.Status.Text() != "" || got.Status.Kind() != "" {
		t.Fatalf("delete status = (%q, %q), want empty", got.Status.Text(), got.Status.Kind())
	}
}
