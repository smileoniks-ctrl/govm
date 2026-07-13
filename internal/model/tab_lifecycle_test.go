package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTabSwitchClearsTabLocalStatus(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Message = "No direct dependency updates available."
	m.MessageType = "warning"
	m.MessageScope = statusScopeTab

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	got := updated.(Model)

	if got.CurrentTab != SettingsTab {
		t.Fatalf("current tab = %d, want %d", got.CurrentTab, SettingsTab)
	}
	if got.Message != "" || got.MessageType != "" {
		t.Fatalf("tab-local status = (%q, %q), want empty", got.Message, got.MessageType)
	}
}

func TestTabSwitchPreservesGlobalStatus(t *testing.T) {
	m := newTestModel(t)
	m.Message = "Successfully installed Go 1.24.4"
	m.MessageType = "success"
	m.MessageScope = statusScopeGlobal

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	got := updated.(Model)

	if got.CurrentTab != InstalledTab {
		t.Fatalf("current tab = %d, want %d", got.CurrentTab, InstalledTab)
	}
	if got.Message != "Successfully installed Go 1.24.4" {
		t.Fatalf("message = %q, want preserved global status", got.Message)
	}
	if got.MessageType != "success" {
		t.Fatalf("message type = %q, want %q", got.MessageType, "success")
	}
}

func TestTabSwitchCancelsPendingDelete(t *testing.T) {
	m := newTestModel(t)
	m.Message = "Are you sure you want to delete Go 1.24.4?"
	m.MessageType = "warning"
	m.MessageScope = statusScopeTab
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
	if got.Message != "" || got.MessageType != "" {
		t.Fatalf("delete status = (%q, %q), want empty", got.Message, got.MessageType)
	}
}
