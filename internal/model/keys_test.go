package model

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
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

func TestAvailableTabArrowKeysMoveListSelection(t *testing.T) {
	m := newTestModel(t)
	m.List.SetItems([]list.Item{
		styles.Item{Name: "1.24.4"},
		styles.Item{Name: "1.25.0"},
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	if got := m.List.Index(); got != 1 {
		t.Fatalf("expected list selection to move down to index 1, got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)

	if got := m.List.Index(); got != 0 {
		t.Fatalf("expected list selection to move up to index 0, got %d", got)
	}
}

func TestInstalledTabArrowKeysMoveTableCursor(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = InstalledTab
	m.Versions = []utils.GoVersion{
		{Version: "1.24.4", Installed: true, Path: "/p/1.24.4"},
		{Version: "1.25.0", Installed: true, Path: "/p/1.25.0"},
	}
	m.rebuildVersionViews()
	m.InstalledTable.Focus()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	if got := m.InstalledTable.Cursor(); got != 1 {
		t.Fatalf("expected installed table cursor to move down to index 1, got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)

	if got := m.InstalledTable.Cursor(); got != 0 {
		t.Fatalf("expected installed table cursor to move up to index 0, got %d", got)
	}
}

func TestRefreshOnDepsTabTriggersCheckCmd(t *testing.T) {
	m := newTestModel(t)

	// Switch to deps tab
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	updated, _ = m.Update(utils.DependenciesMsg{})
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
	updated, _ = m.Update(utils.DependenciesMsg{})
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
	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	// Press 'u'.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	if m.Deps.Dialog.Kind != DialogUpdate {
		t.Fatal("expected update dialog to be open after pressing u on deps tab")
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
	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.0.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	if m.Deps.Dialog.Active() {
		t.Fatal("expected dialog to stay closed when no updates available")
	}
	if m.MessageType != "warning" {
		t.Fatalf("expected warning message, got type %q", m.MessageType)
	}
}
