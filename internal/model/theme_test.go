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

// TestSettingsToggleThemeChangesStateAndMessage replaces the previous
// version that asserted on global state via styles.CurrentTheme(). With
// theme now living on Model as a value, the assertion is that m.theme
// was rebuilt to match the new settings value, with no global state to
// reset in t.Cleanup.
func TestSettingsToggleThemeChangesStateAndMessage(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab
	m.Settings.Cursor = 1
	wantLightPrimary := styles.NewTheme(config.ThemeLight).Primary

	updated, _ := m.Update(tea.KeyPressMsg{Code: ' '})
	m = updated.(Model)

	if m.Settings.Values.Theme != config.ThemeLight {
		t.Fatalf("expected theme light, got %q", m.Settings.Values.Theme)
	}
	if m.Status.Kind() == "error" || m.Status.Text() == "" {
		t.Fatalf("expected non-error message after theme save, got %q: %s", m.Status.Kind(), m.Status.Text())
	}
	if got := m.theme.Primary; got != wantLightPrimary {
		t.Fatalf("expected m.theme to be rebuilt to light; Primary = %v, want %v", got, wantLightPrimary)
	}
}

// TestApplyRuntimeThemeRebuildsDependencyDialogStyles pins the contract
// that previously broke silently (see docs/review/03-performance.md):
// after applyRuntimeTheme the active theme must flow into
// ConfirmDialog.Render. Because Render now takes Theme as a parameter,
// the test also documents the new propagation path explicitly.
func TestApplyRuntimeThemeRebuildsDependencyDialogStyles(t *testing.T) {
	m := newTestModel(t)

	m.Settings.Values.Theme = config.ThemeCurrent
	m.applyRuntimeTheme()
	currentDialog := ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}.
		Render(m.theme, DepsState{}, viewportSize{Width: 64, Height: 20})

	m.Settings.Values.Theme = config.ThemeLight
	m.applyRuntimeTheme()
	lightDialog := ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}.
		Render(m.theme, DepsState{}, viewportSize{Width: 64, Height: 20})

	if lightDialog == currentDialog {
		t.Fatal("expected light theme to change dependency dialog output")
	}
}
