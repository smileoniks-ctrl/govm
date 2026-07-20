package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
