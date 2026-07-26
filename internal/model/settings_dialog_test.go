package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/application"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/loader"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestSettingsDepsBackupLimitDialogOpensWithCurrentValue(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyEnter},
		{Code: ' '},
	} {
		t.Run(key.String(), func(t *testing.T) {
			m := newTestModel(t)
			m.CurrentTab = SettingsTab
			m.Settings.Cursor = 2
			m.Settings.Values.DepsBackupLimit = 25

			updated, _ := m.Update(key)
			m = updated.(Model)

			view := stripANSI(m.View().Content)
			if !strings.Contains(view, "Set dependency backup limit") {
				t.Fatalf("expected backup-limit dialog, got:\n%s", view)
			}
			if !strings.Contains(view, "25") {
				t.Fatalf("expected dialog to contain current value, got:\n%s", view)
			}
			if got := m.Settings.Values.DepsBackupLimit; got != 25 {
				t.Fatalf("backup limit = %d, want unchanged 25", got)
			}
		})
	}
}

func TestSettingsDepsBackupLimitDialogValidatesAndSaves(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 2
	m.Settings.Values.DepsBackupLimit = 10
	if err := config.Save(m.Settings.Path, m.Settings.Values); err != nil {
		t.Fatalf("save initial settings: %v", err)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	for _, invalid := range []string{"", "abc", "0", "101"} {
		m.Settings.DepsBackupLimitInput.SetValue(invalid)
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = updated.(Model)
		if !m.Settings.EditingDepsBackupLimit {
			t.Fatalf("expected dialog to remain open after invalid value %q", invalid)
		}
		if got := m.Settings.Values.DepsBackupLimit; got != 10 {
			t.Fatalf("backup limit after %q = %d, want unchanged 10", invalid, got)
		}
	}

	m.Settings.DepsBackupLimitInput.SetValue("25")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.Settings.EditingDepsBackupLimit {
		t.Fatal("expected dialog to close after valid value")
	}
	if got := m.Settings.Values.DepsBackupLimit; got != 25 {
		t.Fatalf("backup limit = %d, want 25", got)
	}

	data, err := os.ReadFile(m.Settings.Path)
	if err != nil {
		t.Fatalf("read saved settings JSON: %v", err)
	}
	var saved config.Settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshal saved settings JSON: %v", err)
	}
	if got := saved.DepsBackupLimit; got != 25 {
		t.Fatalf("saved backup limit = %d, want 25", got)
	}
}

func TestSettingsDepsBackupLimitDialogCancelsAndBlocksGlobalKeys(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 2
	m.Settings.Values.DepsBackupLimit = 10

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.Settings.DepsBackupLimitInput.SetValue("25")

	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyTab},
		{Code: tea.KeyUp},
		{Code: 'h'},
		{Code: 'q'},
	} {
		updated, cmd := m.Update(key)
		m = updated.(Model)
		if cmd != nil {
			t.Fatalf("key %q returned a global command while dialog was open", key.String())
		}
	}
	if m.CurrentTab != SettingsTab || m.Settings.Cursor != 2 {
		t.Fatalf("global navigation changed while dialog was open: tab=%d cursor=%d", m.CurrentTab, m.Settings.Cursor)
	}
	if got := m.Settings.Values.DepsBackupLimit; got != 10 {
		t.Fatalf("backup limit = %d, want unchanged 10", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.Settings.EditingDepsBackupLimit {
		t.Fatal("expected dialog to close after escape")
	}
	if got := m.Settings.Values.DepsBackupLimit; got != 10 {
		t.Fatalf("backup limit = %d, want unchanged 10", got)
	}
}

func TestSettingsDepsBackupLimitDialogKeepsValueAfterSaveFailure(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 2
	m.Settings.Values.DepsBackupLimit = 10
	m.Settings.Path = t.TempDir()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.Settings.DepsBackupLimitInput.SetValue("25")

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.Settings.EditingDepsBackupLimit {
		t.Fatal("expected dialog to remain open after save failure")
	}
	if got := m.Settings.Values.DepsBackupLimit; got != 10 {
		t.Fatalf("backup limit = %d, want unchanged 10", got)
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "Failed to save settings") {
		t.Fatalf("expected save error in dialog, got:\n%s", view)
	}
}

func TestSettingsDistributionSourceDialogValidatesAndCancels(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 3

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Set distribution source") {
		t.Fatalf("expected distribution source dialog, got:\n%s", view)
	}
	if !strings.Contains(view, config.DefaultDistributionSource) {
		t.Fatalf("expected dialog to contain current source, got:\n%s", view)
	}

	m.Settings.DistributionSourceInput.SetValue("")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("invalid source returned a command")
	}
	if !m.Settings.EditingDistributionSource {
		t.Fatal("expected dialog to remain open after invalid source")
	}
	if m.Settings.Values.DistributionSource != config.DefaultDistributionSource {
		t.Fatalf("source = %q, want unchanged default", m.Settings.Values.DistributionSource)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.Settings.EditingDistributionSource {
		t.Fatal("expected dialog to close after escape")
	}
}

func TestSettingsDistributionSourceUsesOperationResult(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 3
	m = m.BindVersionOperations(VersionOperations{
		DistributionSource: func(context.Context, string) (application.DistributionSourceResult, error) {
			return application.DistributionSourceResult{
				Source: "https://mirror.example/dl/",
				Catalog: loader.VersionCatalog{Versions: []utils.GoVersion{{
					Version:  "1.26.0",
					Filename: "go1.26.0.darwin-arm64.tar.gz",
				}}},
			}, nil
		},
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.Settings.DistributionSourceInput.SetValue("https://mirror.example/dl")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.Settings.EditingDistributionSource || !m.Settings.CheckingDistributionSource {
		t.Fatal("dialog did not remain open while source operation was running")
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.Settings.EditingDistributionSource {
		t.Fatal("dialog remained open after successful source change")
	}
	if m.Settings.Values.DistributionSource != "https://mirror.example/dl/" {
		t.Fatalf("source = %q", m.Settings.Values.DistributionSource)
	}
	if _, ok := m.projection.lookup("1.26.0"); !ok {
		t.Fatal("operation catalog was not applied")
	}
}

func TestSettingsDistributionSourceKeepsDialogOnOperationFailure(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 3
	m = m.BindVersionOperations(VersionOperations{
		DistributionSource: func(context.Context, string) (application.DistributionSourceResult, error) {
			return application.DistributionSourceResult{}, errors.New("catalog unavailable")
		},
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.Settings.DistributionSourceInput.SetValue("https://mirror.example/dl")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if !m.Settings.EditingDistributionSource {
		t.Fatal("dialog closed after failed source change")
	}
	if m.Settings.CheckingDistributionSource {
		t.Fatal("source check remained active after failure")
	}
	if m.Settings.Values.DistributionSource != config.DefaultDistributionSource {
		t.Fatalf("source = %q, want previous source", m.Settings.Values.DistributionSource)
	}
	if !strings.Contains(m.Settings.DistributionSourceInputErr, "catalog unavailable") {
		t.Fatalf("error = %q", m.Settings.DistributionSourceInputErr)
	}
}
