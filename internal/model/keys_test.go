package model

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	coredeps "github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestTabSwitchingCyclesThroughFourTabs(t *testing.T) {
	m := newTestModel(t)

	if m.CurrentTab != AvailableTab {
		t.Fatalf("expected initial tab %d, got %d", AvailableTab, m.CurrentTab)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != InstalledTab {
		t.Fatalf("expected installed tab after first switch, got %d", m.CurrentTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != DepsTab {
		t.Fatalf("expected deps tab after second switch, got %d", m.CurrentTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != SettingsTab {
		t.Fatalf("expected settings tab after third switch, got %d", m.CurrentTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != AvailableTab {
		t.Fatalf("expected available tab after fourth switch, got %d", m.CurrentTab)
	}
}

func TestHandleTabKeyClearsScreenWhenSwitchingToSettings(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab

	updated, cmd := m.handleTabKey()
	if got := updated.(Model).CurrentTab; got != SettingsTab {
		t.Fatalf("current tab = %d, want %d", got, SettingsTab)
	}
	if cmd == nil {
		t.Fatal("expected clear screen command when switching to settings")
	}
	if got, want := reflect.TypeOf(cmd()), reflect.TypeOf(tea.ClearScreen()); got != want {
		t.Fatalf("command message type = %v, want %v", got, want)
	}
}

// shiftTab constructs a KeyPressMsg for Shift+Tab, mirroring the wire
// format produced by bubbletea v2 (KeyTab code + ModShift modifier).
func shiftTab() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
}

func TestShiftTabSwitchesInReverseCycle(t *testing.T) {
	m := newTestModel(t)

	if m.CurrentTab != AvailableTab {
		t.Fatalf("expected initial tab %d, got %d", AvailableTab, m.CurrentTab)
	}

	// Available -> Settings (wraps backwards).
	updated, _ := m.Update(shiftTab())
	m = updated.(Model)
	if m.CurrentTab != SettingsTab {
		t.Fatalf("expected settings tab after first reverse switch, got %d", m.CurrentTab)
	}

	// Settings -> Deps.
	updated, _ = m.Update(shiftTab())
	m = updated.(Model)
	if m.CurrentTab != DepsTab {
		t.Fatalf("expected deps tab after second reverse switch, got %d", m.CurrentTab)
	}

	// Deps -> Installed.
	updated, _ = m.Update(shiftTab())
	m = updated.(Model)
	if m.CurrentTab != InstalledTab {
		t.Fatalf("expected installed tab after third reverse switch, got %d", m.CurrentTab)
	}

	// Installed -> Available.
	updated, _ = m.Update(shiftTab())
	m = updated.(Model)
	if m.CurrentTab != AvailableTab {
		t.Fatalf("expected available tab after fourth reverse switch, got %d", m.CurrentTab)
	}
}

func TestHandleShiftTabKeyClearsScreenWhenSwitchingToSettings(t *testing.T) {
	m := newTestModel(t)
	// On Available tab, reverse navigation wraps to Settings.
	m.CurrentTab = AvailableTab

	updated, cmd := m.handleShiftTabKey()
	if got := updated.(Model).CurrentTab; got != SettingsTab {
		t.Fatalf("current tab = %d, want %d", got, SettingsTab)
	}
	if cmd == nil {
		t.Fatal("expected clear screen command when switching to settings")
	}
	if got, want := reflect.TypeOf(cmd()), reflect.TypeOf(tea.ClearScreen()); got != want {
		t.Fatalf("command message type = %v, want %v", got, want)
	}
}

// TestShiftTabCancelsPendingDelete mirrors the forward-Tab contract:
// switching tabs tears down the pending delete-confirmation context,
// not just for Tab but for the reverse direction too.
func TestShiftTabCancelsPendingDelete(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	m.ConfirmingDelete = true
	m.DeleteVersion = "1.24.4"

	updated, _ := m.handleShiftTabKey()
	m = updated.(Model)

	if m.ConfirmingDelete {
		t.Fatal("expected pending delete confirmation to be cancelled on reverse tab switch")
	}
	if m.DeleteVersion != "" {
		t.Fatalf("expected delete version cleared, got %q", m.DeleteVersion)
	}
	if got, want := m.CurrentTab, AvailableTab; got != want {
		t.Fatalf("current tab = %d, want %d (reverse of Installed)", got, want)
	}
}

// TestConfirmDialogShiftTabTogglesChoice mirrors Tab inside the Deps
// confirm dialog: Shift+Tab flips the Yes/No selection rather than
// escaping the dialog, matching desktop conventions for 2-button dialogs.
func TestConfirmDialogShiftTabTogglesChoice(t *testing.T) {
	d := ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}

	got, action := d.Handle(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if action != DialogNoop {
		t.Fatalf("expected DialogNoop, got %v", action)
	}
	if got.ChoiceYes {
		t.Fatal("expected Shift+Tab to flip ChoiceYes from true to false")
	}

	got, _ = got.Handle(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !got.ChoiceYes {
		t.Fatal("expected second Shift+Tab to flip ChoiceYes back to true")
	}
}

func TestAvailableTabArrowKeysMoveListSelection(t *testing.T) {
	m := newTestModel(t)
	seedVersions(t, &m, []utils.GoVersion{
		{Version: "1.24.4"},
		{Version: "1.25.0"},
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	if got := m.projection.availableModel().Index(); got != 1 {
		t.Fatalf("expected list selection to move down to index 1, got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)

	if got := m.projection.availableModel().Index(); got != 0 {
		t.Fatalf("expected list selection to move up to index 0, got %d", got)
	}
}

func TestInstalledTabArrowKeysMoveTableCursor(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	seedVersions(t, &m, []utils.GoVersion{
		{Version: "1.24.4", Installed: true, Path: "/p/1.24.4"},
		{Version: "1.25.0", Installed: true, Path: "/p/1.25.0"},
	})
	focusInstalled(&m)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	if got := m.projection.installedModel().Cursor(); got != 1 {
		t.Fatalf("expected installed table cursor to move down to index 1, got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)

	if got := m.projection.installedModel().Cursor(); got != 0 {
		t.Fatalf("expected installed table cursor to move up to index 0, got %d", got)
	}
}

func TestInstalledTabDeleteUsesTableSelection(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	seedVersions(t, &m, []utils.GoVersion{
		{Version: "1.24.4", Installed: true, Path: "/p/1.24.4"},
		{Version: "1.25.0", Installed: true, Path: "/p/1.25.0"},
	})
	setInstalledCursor(&m, 1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd'})
	got := updated.(Model)
	if !got.ConfirmingDelete {
		t.Fatal("expected delete confirmation")
	}
	if got.DeleteVersion != "1.25.0" {
		t.Fatalf("delete version = %q, want 1.25.0", got.DeleteVersion)
	}
}

func TestRefreshOnDepsTabTriggersCheckCmd(t *testing.T) {
	m := newTestModel(t)

	// Switch to deps tab
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	updated, _ = m.Update(DependenciesMsg{})
	m = updated.(Model)

	// Press 'r' on deps tab
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)

	if m.Deps.Phase != OpChecking {
		t.Fatal("expected CheckingDependencies to be true after pressing r on deps tab")
	}

	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
}

func TestPressBOnDepsTabTriggersBackupListCmd(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	updated, _ = m.Update(DependenciesMsg{})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'b'})
	m = updated.(Model)

	if m.Deps.Phase != OpLoadingBackups {
		t.Fatal("expected LoadingBackups to be true after pressing b on deps tab")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
}

func TestPressUOnDepsOpensConfirmDialog(t *testing.T) {
	m := newTestModel(t)

	// Switch to deps tab.
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	// Load deps with one direct update.
	deps := DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	// Press 'u'.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)
	updated, _ = m.Update(coredeps.CheckUpdatesDoneEvent{Dependencies: []coredeps.ModuleDependency(deps)})
	m = updated.(Model)

	if m.Deps.Dialog.Kind != DialogUpdate {
		t.Fatal("expected update dialog after fresh preflight")
	}
	if !m.Deps.Dialog.ChoiceYes {
		t.Fatal("expected default choice to be Yes")
	}
}

func TestPressUOnDepsWithoutUpdatesShowsMessage(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	// Load deps with no updates.
	deps := DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.0.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)
	updated, _ = m.Update(coredeps.CheckUpdatesDoneEvent{Dependencies: []coredeps.ModuleDependency(deps)})
	m = updated.(Model)

	if m.Deps.Dialog.Active() {
		t.Fatal("expected dialog to stay closed when no updates available")
	}
	if m.Status.Kind() != "warning" {
		t.Fatalf("expected warning message, got type %q", m.Status.Kind())
	}
}
