package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestSettingsDepsBackupLimitShortcutControlsAndSaves(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		start int
		want  int
	}{
		{name: "left wraps minimum to maximum", key: tea.KeyPressMsg{Code: tea.KeyLeft}, start: config.MinDepsBackupLimit, want: config.MaxDepsBackupLimit},
		{name: "right wraps maximum to minimum", key: tea.KeyPressMsg{Code: tea.KeyRight}, start: config.MaxDepsBackupLimit, want: config.MinDepsBackupLimit},
		{name: "h wraps minimum to maximum", key: tea.KeyPressMsg{Code: 'h'}, start: config.MinDepsBackupLimit, want: config.MaxDepsBackupLimit},
		{name: "l wraps maximum to minimum", key: tea.KeyPressMsg{Code: 'l'}, start: config.MaxDepsBackupLimit, want: config.MinDepsBackupLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.CurrentTab = SettingsTab
			m.Settings.Cursor = 2
			m.Settings.Values.DepsBackupLimit = tt.start

			updated, _ := m.Update(tt.key)
			m = updated.(Model)
			if got := m.Settings.Values.DepsBackupLimit; got != tt.want {
				t.Fatalf("backup limit after %q = %d, want %d", tt.key.String(), got, tt.want)
			}

			data, err := os.ReadFile(m.Settings.Path)
			if err != nil {
				t.Fatalf("read saved settings JSON: %v", err)
			}
			var saved config.Settings
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatalf("unmarshal saved settings JSON: %v", err)
			}
			if got := saved.DepsBackupLimit; got != tt.want {
				t.Fatalf("saved JSON backup limit after %q = %d, want %d", tt.key.String(), got, tt.want)
			}
		})
	}
}

func TestSettingsToggleDepsDisplayUpdatesDependencyRows(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	updated, _ := m.Update(utils.DependenciesMsg{
		{Path: "github.com/example/direct", Version: "v1.0.0"},
		{Path: "github.com/example/indirect", Version: "v1.0.0", Indirect: true},
	})
	m = updated.(Model)

	if rows := m.Deps.Table.Rows(); len(rows) != 1 {
		t.Fatalf("expected default direct-only view to show 1 row, got %d", len(rows))
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Settings.Values.DepsDisplay != config.DepsDisplayAll {
		t.Fatalf("expected deps display all, got %q", m.Settings.Values.DepsDisplay)
	}
	if rows := m.Deps.Table.Rows(); len(rows) != 2 {
		t.Fatalf("expected all deps view to show 2 rows, got %d", len(rows))
	}
	if m.MessageType == "error" {
		t.Fatalf("expected non-error message after save, got %q: %s", m.MessageType, m.Message)
	}
}

func TestSettingsSaveErrorShowsErrorMessage(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Path = t.TempDir()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.MessageType != "error" {
		t.Fatalf("expected error message type, got %q", m.MessageType)
	}
	if !strings.Contains(m.Message, "settings") {
		t.Fatalf("expected settings save error message, got %q", m.Message)
	}
}
