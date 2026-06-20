package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/melkeydev/govm/internal/styles"
	"github.com/melkeydev/govm/internal/utils"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func newTestModel(t *testing.T) Model {
	t.Helper()

	home := t.TempDir()
	shim := filepath.Join(home, ".govm", "shim")
	if err := os.MkdirAll(shim, 0755); err != nil {
		t.Fatalf("create shim dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", shim)

	items := []list.Item{
		styles.Item{
			Name:            "1.24.4",
			DescriptionText: "go1.24.4.darwin-arm64.tar.gz",
			Installed:       true,
			Active:          true,
		},
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 10)
	l.SetShowHelp(false)
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "Version", Width: 12},
			{Title: "Path", Width: 32},
			{Title: "Status", Width: 12},
		}),
		table.WithHeight(8),
	)

	return Model{
		List:           l,
		Versions:       []utils.GoVersion{{Version: "1.24.4", Filename: "go1.24.4.darwin-arm64.tar.gz", Installed: true, Active: true, Path: filepath.Join(home, ".govm", "versions", "go1.24.4")}},
		Spinner:        spinner.New(),
		HomeDir:        home,
		GoVersionsDir:  filepath.Join(home, ".govm", "versions"),
		InstalledTable: tbl,
		Message:        "Successfully installed Go 1.24.4",
		MessageType:    "success",
		Layout:         styles.LayoutWide,
	}
}

func TestViewUsesModernZones(t *testing.T) {
	m := newTestModel(t)

	view := stripANSI(m.View().Content)

	for _, want := range []string{"GoVM", "Go Version Manager", "● Available", "○ Installed", "✓ Successfully installed Go 1.24.4", "i install", "u use", "d delete", "r refresh", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}

	if strings.Contains(view, "Press 'i'") || strings.Contains(view, "[ Available Versions ]") {
		t.Fatalf("expected modern tabs and help text, got:\n%s", view)
	}
}

func TestWindowSizeMsgKeepsContentSizesPositive(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	got := updated.(Model)

	if got.List.Width() <= 0 || got.List.Height() <= 0 {
		t.Fatalf("expected positive list size, got %dx%d", got.List.Width(), got.List.Height())
	}

	if got.InstalledTable.Width() <= 0 || got.InstalledTable.Height() <= 0 {
		t.Fatalf("expected positive table size, got %dx%d", got.InstalledTable.Width(), got.InstalledTable.Height())
	}
}

func TestTabSwitchingCyclesThroughThreeTabs(t *testing.T) {
	m := newTestModel(t)

	if m.CurrentTab != 0 {
		t.Fatalf("expected initial tab 0, got %d", m.CurrentTab)
	}

	// tab → 1
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != 1 {
		t.Fatalf("expected tab 1 after first switch, got %d", m.CurrentTab)
	}

	// tab → 2
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != 2 {
		t.Fatalf("expected tab 2 after second switch, got %d", m.CurrentTab)
	}

	// tab → 0
	updated, _ = m.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	if m.CurrentTab != 0 {
		t.Fatalf("expected tab 0 after third switch, got %d", m.CurrentTab)
	}
}

func TestDepsTabRenders(t *testing.T) {
	m := newTestModel(t)

	// Switch to deps tab
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	view := stripANSI(m.View().Content)

	if !strings.Contains(view, "Deps") {
		t.Fatalf("expected deps tab label in view, got:\n%s", view)
	}

	if !strings.Contains(view, "check updates") {
		t.Fatalf("expected 'check updates' help hint, got:\n%s", view)
	}
}

func TestWindowSizeMsgResizesDepsTable(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	got := updated.(Model)

	if got.DependencyTable.Width() <= 0 || got.DependencyTable.Height() <= 0 {
		t.Fatalf("expected positive deps table size, got %dx%d", got.DependencyTable.Width(), got.DependencyTable.Height())
	}
}

func TestRefreshOnDepsTabTriggersCheckCmd(t *testing.T) {
	m := newTestModel(t)

	// Switch to deps tab
	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	// Press 'r' on deps tab
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)

	if !m.CheckingDependencies {
		t.Fatal("expected CheckingDependencies to be true after pressing r on deps tab")
	}

	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
}

func TestDependenciesMsgPopulatesTable(t *testing.T) {
	m := newTestModel(t)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
		{Path: "github.com/example/indirect", Version: "v0.5.0", Indirect: true},
		{Path: "github.com/example/current", Version: "v2.0.0", Latest: "v2.0.0"},
	}

	updated, _ := m.Update(deps)
	got := updated.(Model)

	if !got.DependenciesLoaded {
		t.Fatal("expected DependenciesLoaded to be true")
	}

	if len(got.Dependencies) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(got.Dependencies))
	}

	rows := got.DependencyTable.Rows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows in table, got %d", len(rows))
	}

	// Check statuses
	if rows[0][3] != "update avail" {
		t.Fatalf("expected 'update avail' status, got %q", rows[0][3])
	}
	if rows[1][3] != "indirect" {
		t.Fatalf("expected 'indirect' status, got %q", rows[1][3])
	}
	if rows[2][3] != "current" {
		t.Fatalf("expected 'current' status, got %q", rows[2][3])
	}
}

func TestDependencyTableColumns(t *testing.T) {
	cols := dependencyTableColumns(60, styles.LayoutCompact)

	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}

	if cols[0].Width < 5 || cols[1].Width < 3 || cols[2].Width < 3 || cols[3].Width < 3 {
		t.Fatal("expected positive column widths in compact mode")
	}
}

func TestUpdatableDirectDependenciesFilter(t *testing.T) {
	deps := []utils.ModuleDependency{
		{Path: "direct-updatable", Version: "v1.0.0", Latest: "v1.1.0"},
		{Path: "indirect-updatable", Version: "v0.5.0", Latest: "v0.6.0", Indirect: true},
		{Path: "direct-current", Version: "v2.0.0", Latest: "v2.0.0"},
		{Path: "direct-no-info", Version: "v3.0.0"},
		{Path: "direct-error", Version: "v4.0.0", Latest: "v4.1.0", Error: "bad module"},
	}

	updatable := utils.UpdatableDirectDependencies(deps)

	if len(updatable) != 1 {
		t.Fatalf("expected 1 updatable direct dep, got %d (%v)", len(updatable), updatable)
	}
	if updatable[0].Path != "direct-updatable" {
		t.Fatalf("expected direct-updatable, got %q", updatable[0].Path)
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

	if !m.ConfirmingDependencyUpdate {
		t.Fatal("expected ConfirmingDependencyUpdate to be true after pressing u on deps tab")
	}
	if !m.UpdateChoiceYes {
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

	if m.ConfirmingDependencyUpdate {
		t.Fatal("expected dialog to stay closed when no updates available")
	}
	if m.MessageType != "warning" {
		t.Fatalf("expected warning message, got type %q", m.MessageType)
	}
}

func TestEscClosesConfirmDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if m.ConfirmingDependencyUpdate {
		t.Fatal("expected dialog to close on esc")
	}
}

func TestRightArrowTogglesDialogChoice(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	if !m.UpdateChoiceYes {
		t.Fatal("expected default to be Yes")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(Model)

	if m.UpdateChoiceYes {
		t.Fatal("expected right arrow to toggle choice to No")
	}
}

func TestConfirmOnNoClosesDialogWithoutUpdate(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // toggle to No
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.ConfirmingDependencyUpdate {
		t.Fatal("expected dialog to close after confirm on No")
	}
	if m.UpdatingDependencies {
		t.Fatal("expected UpdatingDependencies to be false after choosing No")
	}
}

func TestConfirmOnYesTriggersUpdateCmd(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	deps := utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	}
	updated, _ = m.Update(deps)
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u'})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.ConfirmingDependencyUpdate {
		t.Fatal("expected dialog to close after confirm on Yes")
	}
	if !m.UpdatingDependencies {
		t.Fatal("expected UpdatingDependencies to be true after choosing Yes")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned after confirming Yes")
	}
}

func TestDependenciesUpdatedMsgUpdatesState(t *testing.T) {
	m := newTestModel(t)

	msg := utils.DependenciesUpdatedMsg{
		Updated: 2,
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.1.0", Latest: "v1.1.0"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.UpdatingDependencies {
		t.Fatal("expected UpdatingDependencies to be false after update complete")
	}
	if len(got.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got type %q", got.MessageType)
	}
}

func TestDependencyTableIndirectUpdateStatus(t *testing.T) {
	m := newTestModel(t)

	deps := utils.DependenciesMsg{
		{Path: "indirect-with-update", Version: "v0.5.0", Latest: "v0.6.0", Indirect: true},
	}

	updated, _ := m.Update(deps)
	got := updated.(Model)

	rows := got.DependencyTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][3] != "indirect update" {
		t.Fatalf("expected 'indirect update' status, got %q", rows[0][3])
	}
}

func TestRenderDependencyUpdateDialogContainsWarning(t *testing.T) {
	dialog := stripANSI(renderDependencyUpdateDialog(true))

	for _, want := range []string{"Warning", "Вы уверены", "Да", "Нет"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("expected dialog to contain %q, got:\n%s", want, dialog)
		}
	}
}

func TestDialogRendersOverDepsView(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(utils.DependenciesMsg{
		{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
	})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)

	view := stripANSI(m.View().Content)

	if !strings.Contains(view, "Warning") {
		t.Fatal("expected warning text in view when dialog is open")
	}
	if !strings.Contains(view, "Deps") {
		t.Fatal("expected deps tab content to still be visible behind dialog")
	}
}
