package model

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

func TestSettingsDepsBackupLimitInputFollowsTheme(t *testing.T) {
	m := newTestModel(t)

	m.Settings.Values.Theme = config.ThemeLight
	m.applyRuntimeTheme()
	if got := m.Settings.DepsBackupLimitInput.Styles(); !reflect.DeepEqual(got, textinput.DefaultLightStyles()) {
		t.Fatal("expected backup limit input to use light theme styles")
	}

	m.Settings.Values.Theme = config.ThemeCurrent
	m.applyRuntimeTheme()
	if got := m.Settings.DepsBackupLimitInput.Styles(); !reflect.DeepEqual(got, textinput.DefaultDarkStyles()) {
		t.Fatal("expected backup limit input to use current theme styles")
	}
}

func TestSettingsToggleThemeChangesStateAndMessage(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme(styles.ThemeCurrent)
	})
	ApplyTheme(styles.ThemeCurrent)
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 1

	updated, _ := m.Update(tea.KeyPressMsg{Code: ' '})
	m = updated.(Model)

	if m.Settings.Values.Theme != config.ThemeLight {
		t.Fatalf("expected theme light, got %q", m.Settings.Values.Theme)
	}
	if m.MessageType == "error" || m.Message == "" {
		t.Fatalf("expected non-error message after theme save, got %q: %s", m.MessageType, m.Message)
	}
	if got := styles.CurrentTheme(); got != styles.ThemeLight {
		t.Fatalf("expected runtime theme light, got %q", got)
	}
}

func TestApplyThemeRebuildsDependencyDialogStyles(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme(styles.ThemeCurrent)
	})

	ApplyTheme(styles.ThemeCurrent)
	currentDialog := renderDependencyChecksDialog(true, viewportSize{Width: 64, Height: 20})

	if got := ApplyTheme(styles.ThemeLight); got != styles.ThemeLight {
		t.Fatalf("expected light theme, got %q", got)
	}

	lightDialog := renderDependencyChecksDialog(true, viewportSize{Width: 64, Height: 20})
	if lightDialog == currentDialog {
		t.Fatal("expected light theme to change dependency dialog output")
	}
}
