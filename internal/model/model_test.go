package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
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

	prev := utils.Version
	utils.Version = "v9.9.9-test"
	defer func() { utils.Version = prev }()

	view := stripANSI(m.View().Content)

	for _, want := range []string{"GoVM", "Go Version Manager", "v9.9.9-test", "● Available", "○ Installed", "✓ Successfully installed Go 1.24.4", "i install", "u use", "d delete", "r refresh", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}

	if strings.Contains(view, "Press 'i'") || strings.Contains(view, "[ Available Versions ]") {
		t.Fatalf("expected modern tabs and help text, got:\n%s", view)
	}
}

func TestGoDevErrorKeepsTUIClosable(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(utils.ErrMsg(errors.New("failed to connect to go.dev: context deadline exceeded")))
	m = updated.(Model)

	view := stripANSI(m.View().Content)

	for _, want := range []string{"GoVM", "Available", "failed to connect to go.dev", "r refresh", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
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
	dialog := stripANSI(renderDependencyUpdateDialog(true, nil))

	for _, want := range []string{"Warning", "Будут обновлены", "Да", "Нет"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("expected dialog to contain %q, got:\n%s", want, dialog)
		}
	}
}

func TestRenderDependencyUpdateDialogListsModules(t *testing.T) {
	entries := []utils.DependencyUpdateEntry{
		{Path: "github.com/example/lib", OldVersion: "v1.0.0", NewVersion: "v1.1.0"},
		{Path: "github.com/example/other", OldVersion: "v2.0.0", NewVersion: "v2.1.0"},
	}
	dialog := stripANSI(renderDependencyUpdateDialog(true, entries))

	for _, want := range []string{"github.com/example/lib", "v1.0.0", "v1.1.0", "github.com/example/other"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("expected dialog to contain %q, got:\n%s", want, dialog)
		}
	}
}

func TestRenderDependencyUpdateDialogTruncatesLongLists(t *testing.T) {
	entries := make([]utils.DependencyUpdateEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, utils.DependencyUpdateEntry{
			Path:       fmt.Sprintf("github.com/example/dep%d", i),
			OldVersion: "v1.0.0",
			NewVersion: "v1.1.0",
		})
	}
	dialog := stripANSI(renderDependencyUpdateDialog(true, entries))

	if !strings.Contains(dialog, "и ещё") {
		t.Fatalf("expected truncation hint in dialog, got:\n%s", dialog)
	}
}

func TestRenderDependencyChecksDialogContainsCommands(t *testing.T) {
	dialog := stripANSI(renderDependencyChecksDialog(true))

	for _, want := range []string{"Запустить проверки", "go test", "go vet", "Да", "Нет"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("expected dialog to contain %q, got:\n%s", want, dialog)
		}
	}
}

func TestRenderDependencyRollbackDialogContainsCommand(t *testing.T) {
	result := &utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL: example_test.go:10: expected 1, got 2",
	}
	dialog := stripANSI(renderDependencyRollbackDialog(true, result))

	for _, want := range []string{"Проверки провалились", "go test ./...", "FAIL: example_test", "Откатить", "Оставить"} {
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
	// Regression guard: the actual dependency row must still be rendered
	// somewhere on screen above or below the modal. Previously the dialog
	// erased the whole deps table.
	if !strings.Contains(view, "github.com/example/lib") {
		t.Fatalf("expected dependency row to remain visible when confirm dialog is open, got:\n%s", view)
	}
}

func TestInstalledTableColumns_AllLayouts(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		layout styles.LayoutMode
	}{
		{"compact-min", 20, styles.LayoutCompact},
		{"compact-wide", 120, styles.LayoutCompact},
		{"normal", 100, styles.LayoutNormal},
		{"wide", 160, styles.LayoutWide},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols := installedTableColumns(tc.width, tc.layout)
			if len(cols) != 3 {
				t.Fatalf("expected 3 columns, got %d", len(cols))
			}
			for i, c := range cols {
				if c.Width <= 0 {
					t.Fatalf("column %d (%s) has non-positive width %d", i, c.Title, c.Width)
				}
			}
		})
	}
}

func TestDependencyTableColumns_AllLayouts(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		layout styles.LayoutMode
	}{
		{"compact-min", 20, styles.LayoutCompact},
		{"compact-wide", 120, styles.LayoutCompact},
		{"normal", 100, styles.LayoutNormal},
		{"wide", 160, styles.LayoutWide},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols := dependencyTableColumns(tc.width, tc.layout)
			if len(cols) != 4 {
				t.Fatalf("expected 4 columns, got %d", len(cols))
			}
			for i, c := range cols {
				if c.Width < 5 {
					t.Fatalf("column %d (%s) too narrow: %d", i, c.Title, c.Width)
				}
			}
		})
	}
}

func TestUpdateDependencyTable_StatusPriorities(t *testing.T) {
	m := newTestModel(t)

	deps := utils.DependenciesMsg{
		{Path: "err", Version: "v1.0.0", Latest: "v1.1.0", Error: "boom"},
		{Path: "dep", Version: "v1.0.0", Latest: "v1.1.0", Deprecated: "use v2"},
		{Path: "indirect-update", Version: "v1.0.0", Latest: "v1.1.0", Indirect: true},
		{Path: "direct-update", Version: "v1.0.0", Latest: "v1.1.0"},
		{Path: "indirect-only", Version: "v1.0.0", Indirect: true},
		{Path: "current", Version: "v1.0.0", Latest: "v1.0.0"},
	}
	updated, _ := m.Update(deps)
	got := updated.(Model)
	rows := got.DependencyTable.Rows()
	if len(rows) != len(deps) {
		t.Fatalf("expected %d rows, got %d", len(deps), len(rows))
	}
	want := []string{"error", "deprecated", "indirect update", "update avail", "indirect", "current"}
	for i, w := range want {
		if rows[i][3] != w {
			t.Fatalf("row %d: expected %q, got %q", i, w, rows[i][3])
		}
	}
}

func TestUpdateInstalledTable_SkipsUninstalled(t *testing.T) {
	m := newTestModel(t)
	m.Versions = []utils.GoVersion{
		{Version: "1.20.0", Installed: true, Path: "/p/1.20", Active: true},
		{Version: "1.21.0", Installed: false},
		{Version: "1.22.0", Installed: true, Path: "/p/1.22"},
	}
	m.updateInstalledTable()
	rows := m.InstalledTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 installed rows, got %d", len(rows))
	}
	if rows[0][0] != "1.20.0" || rows[0][2] != "active" {
		t.Fatalf("row 0 mismatch: %v", rows[0])
	}
	if rows[1][0] != "1.22.0" || rows[1][2] != "" {
		t.Fatalf("row 1 mismatch: %v", rows[1])
	}
}

func TestSpliceCentered_Basic(t *testing.T) {
	got := spliceCentered("hello world", "ABC", 3)
	want := "helABCworld"
	if got != want {
		t.Fatalf("spliceCentered: got %q, want %q", got, want)
	}
}

func TestSpliceCentered_EdgeCases(t *testing.T) {
	cases := []struct {
		name        string
		bg, overlay string
		col         int
		want        string
	}{
		{"overlay-shorter-than-bg", "abcdef", "XY", 1, "aXYdef"},
		{"col-zero", "abcdef", "XY", 0, "XYcdef"},
		{"col-negative-clamped", "abcdef", "XY", -5, "XYcdef"},
		{"col-beyond-bg-clamped", "abc", "XYZ", 100, "abcXYZ"},
		{"col-at-end", "abc", "XY", 3, "abcXY"},
		{"unicode-runes", "abcdefgh", "OK", 3, "abcOKfgh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := spliceCentered(tc.bg, tc.overlay, tc.col)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPluralize(t *testing.T) {
	if pluralize(1, "dep", "deps") != "dep" {
		t.Fatal("expected singular for n=1")
	}
	if pluralize(0, "dep", "deps") != "deps" {
		t.Fatal("expected plural for n=0")
	}
	if pluralize(5, "dep", "deps") != "deps" {
		t.Fatal("expected plural for n>1")
	}
}

func TestMaxInt(t *testing.T) {
	if maxInt(3, 7) != 7 {
		t.Fatal("expected 7")
	}
	if maxInt(7, 3) != 7 {
		t.Fatal("expected 7")
	}
	if maxInt(4, 4) != 4 {
		t.Fatal("expected 4")
	}
}

func TestOverlayDialog_ReplacesCenterRegion(t *testing.T) {
	bg := strings.Repeat("line\n", 9) + "line"
	dlg := "AAA\nBBB\nCCC"
	out := overlayDialog(bg, dlg, 20, 10)
	stripped := stripANSI(out)
	for _, want := range []string{"AAA", "BBB", "CCC"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, out)
		}
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines preserved, got %d", len(lines))
	}
}

func TestOverlayDialog_ClampsToSize(t *testing.T) {
	bg := strings.Repeat("bg\n", 15) + "bg"
	dlg := "VISIBLE"
	out := overlayDialog(bg, dlg, 0, 0)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "VISIBLE") {
		t.Fatalf("expected dialog content in output, got:\n%s", out)
	}
}

// TestOverlayDialog_PreservesRowsOutsideDialog guards against the regression
// captured in CleanShot 2026-06-27 at 17.54.47@2x.png, where the dependency
// update confirmation dialog erased most of the deps table because
// overlayDialog built a full-height canvas and overwrote every row, even the
// ones outside the actual modal box.
func TestOverlayDialog_PreservesRowsOutsideDialog(t *testing.T) {
	// 20 background rows, each tagged with a unique marker. The dialog is
	// only 3 rows tall, so rows 0..7 and 11..19 must survive untouched.
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("BG_ROW_%02d", i))
	}
	bg := strings.Join(lines, "\n")
	dlg := "AAA\nBBB\nCCC"

	out := overlayDialog(bg, dlg, 30, 20)
	stripped := stripANSI(out)
	strippedLines := strings.Split(stripped, "\n")

	// Find which lines contain dialog content.
	overwritten := 0
	for _, l := range strippedLines {
		if strings.Contains(l, "AAA") || strings.Contains(l, "BBB") || strings.Contains(l, "CCC") {
			overwritten++
		}
	}
	if overwritten > 3 {
		t.Fatalf("overlayDialog overwrote %d background rows with dialog content; expected at most 3:\n%s", overwritten, stripped)
	}

	// Count the surviving background markers.
	survivors := 0
	for _, marker := range []string{
		"BG_ROW_00", "BG_ROW_01", "BG_ROW_02", "BG_ROW_03", "BG_ROW_04",
		"BG_ROW_15", "BG_ROW_16", "BG_ROW_17", "BG_ROW_18", "BG_ROW_19",
	} {
		if strings.Contains(stripped, marker) {
			survivors++
		}
	}
	if survivors < 8 {
		t.Fatalf("expected at least 8 background rows preserved outside the dialog, got %d:\n%s", survivors, stripped)
	}
}

// TestSpliceCentered_UsesVisibleColumnsWithANSI guards against spliceCentered
// slicing by rune count when col is measured in visible cells, which used to
// break ANSI escape sequences in styled table output.
func TestSpliceCentered_UsesVisibleColumnsWithANSI(t *testing.T) {
	// A styled background line where the visible content is 9 cells but the
	// raw string contains ANSI escape sequences that pad it to many more
	// bytes/runes.
	styled := "\x1b[31mhello    \x1b[0m" // 9 visible cells: h e l l o _ _ _ _
	overlay := "X"
	// col is measured in visible cells; placing at col=4 must REPLACE the
	// "o" at cell 4 with "X" and keep the trailing 4 spaces plus the
	// surrounding ANSI codes intact.
	got := spliceCentered(styled, overlay, 4)

	if w := ansi.StringWidth(got); w != 9 {
		t.Fatalf("expected result width 9, got %d (raw: %q)", w, got)
	}
	plain := stripANSI(got)
	if !strings.HasPrefix(plain, "hellX") {
		t.Fatalf("expected plain output to start with %q, got %q", "hellX", plain)
	}
	if !strings.HasSuffix(plain, "    ") {
		t.Fatalf("expected plain output to end with 4 spaces, got %q", plain)
	}
	if !strings.Contains(got, "\x1b[31m") {
		t.Fatalf("expected ANSI opening sequence to be preserved, got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("expected ANSI reset sequence to be preserved, got %q", got)
	}
}

func TestRenderHelp_ConfirmsDeleteVariant(t *testing.T) {
	got := renderHelp(0, true, false, false, false, 80, styles.LayoutNormal)
	if !strings.Contains(stripANSI(got), "confirm") {
		t.Fatalf("expected confirm hint, got: %s", got)
	}
	if !strings.Contains(stripANSI(got), "cancel") {
		t.Fatalf("expected cancel hint, got: %s", got)
	}
}

func TestRenderHelp_DepsCompactTruncates(t *testing.T) {
	got := renderHelp(2, false, false, false, false, 20, styles.LayoutCompact)
	if got == "" {
		t.Fatal("expected non-empty help for deps compact")
	}
}

func TestRenderStatus_EmptyMessage(t *testing.T) {
	if renderStatus("info", "", 80) != "" {
		t.Fatal("expected empty result for empty message")
	}
}

func TestRenderStatus_AllTypes(t *testing.T) {
	types := []string{"success", "error", "warning", "info", "unknown"}
	for _, ty := range types {
		got := renderStatus(ty, "msg", 80)
		if !strings.Contains(stripANSI(got), "msg") {
			t.Fatalf("status type %q should include message, got: %s", ty, got)
		}
	}
}

func TestView_NoPanicWhenListEmpty(t *testing.T) {
	m := newTestModel(t)
	m.List.SetItems([]list.Item{})
	m.Versions = nil
	view := m.View()
	if view.Content == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestDependenciesUpdatedMsgStoresSnapshotAndOpensChecksDialog(t *testing.T) {
	m := newTestModel(t)

	msg := utils.DependenciesUpdatedMsg{
		Updated: 1,
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.1.0", Latest: "v1.1.0"},
		},
		Snapshot: &utils.DependencySnapshot{
			ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
			SumFile: utils.ModuleFileSnapshot{Exists: true, Content: "oldsum"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.UpdatingDependencies {
		t.Fatal("expected UpdatingDependencies to be false")
	}
	if got.LastDependencySnapshot == nil {
		t.Fatal("expected LastDependencySnapshot to be set")
	}
	if !got.ConfirmingDependencyChecks {
		t.Fatal("expected ConfirmingDependencyChecks to be true")
	}
	if !got.CheckChoiceYes {
		t.Fatal("expected CheckChoiceYes default to be Yes")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got %q", got.MessageType)
	}
}

func TestDependencyCheckResultOKClearsDialog(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyChecks = true
	m.CheckChoiceYes = true

	updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true})
	got := updated.(Model)

	if got.ConfirmingDependencyChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close after success")
	}
	if got.RunningDependencyChecks {
		t.Fatal("expected RunningDependencyChecks to be false")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
	if got.LastCheckResult == nil {
		t.Fatal("expected LastCheckResult to be stored")
	}
}

func TestDependencyCheckResultFailOpensRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyChecks = true
	m.CheckChoiceYes = true
	m.RunningDependencyChecks = true

	msg := utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL",
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.ConfirmingDependencyChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close on failure")
	}
	if !got.ConfirmingDependencyRollback {
		t.Fatal("expected ConfirmingDependencyRollback to be true")
	}
	if !got.RollbackChoiceYes {
		t.Fatal("expected RollbackChoiceYes default to be Yes")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
	if got.LastCheckResult == nil || got.LastCheckResult.Command != "go test ./..." {
		t.Fatalf("expected LastCheckResult to capture failing command, got %+v", got.LastCheckResult)
	}
}

func TestRollbackCmdTriggeredByRollbackYes(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyRollback = true
	m.RollbackChoiceYes = true
	m.LastDependencySnapshot = &utils.DependencySnapshot{
		ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
		SumFile: utils.ModuleFileSnapshot{Exists: true, Content: "oldsum"},
	}
	m.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test ./..."}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.ConfirmingDependencyRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close")
	}
	if !got.RollingBackDependencies {
		t.Fatal("expected RollingBackDependencies to be true")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned for rollback")
	}
}

func TestKeepCmdClearsRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyRollback = true
	m.RollbackChoiceYes = true
	m.LastDependencySnapshot = &utils.DependencySnapshot{}
	m.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test"}

	// Toggle to No then confirm.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.ConfirmingDependencyRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close when keeping updates")
	}
	if got.RollingBackDependencies {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
	if got.MessageType != "warning" {
		t.Fatalf("expected warning status, got %q", got.MessageType)
	}
}

func TestEscOnChecksDialogSkipsChecks(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyChecks = true
	m.CheckChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.ConfirmingDependencyChecks {
		t.Fatal("expected dialog to close on esc")
	}
	if got.RunningDependencyChecks {
		t.Fatal("expected RunningDependencyChecks to remain false")
	}
}

func TestEscOnRollbackDialogKeepsUpdates(t *testing.T) {
	m := newTestModel(t)
	m.ConfirmingDependencyRollback = true
	m.RollbackChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.ConfirmingDependencyRollback {
		t.Fatal("expected dialog to close on esc")
	}
	if got.RollingBackDependencies {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
}

func TestDependenciesRolledBackMsgUpdatesState(t *testing.T) {
	m := newTestModel(t)
	m.RollingBackDependencies = true
	m.LastDependencySnapshot = &utils.DependencySnapshot{}

	msg := utils.DependenciesRolledBackMsg{
		Snapshot: &utils.DependencySnapshot{
			ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
		},
		Dependencies: []utils.ModuleDependency{
			{Path: "github.com/example/lib", Version: "v1.0.0", Latest: "v1.1.0"},
		},
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.RollingBackDependencies {
		t.Fatal("expected RollingBackDependencies to be false")
	}
	if len(got.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
}

func TestDependencyErrDuringRollbackClearsState(t *testing.T) {
	m := newTestModel(t)
	m.RollingBackDependencies = true

	updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("boom")})
	got := updated.(Model)

	if got.RollingBackDependencies {
		t.Fatal("expected RollingBackDependencies to be false after err")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
}

func TestViewShowsRollbackDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)

	m.ConfirmingDependencyRollback = true
	m.RollbackChoiceYes = true
	m.LastCheckResult = &utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL: foo_test.go:42",
	}

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Проверки провалились") {
		t.Fatalf("expected rollback dialog in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Откатить") {
		t.Fatal("expected rollback button in view")
	}
}

func TestViewShowsChecksDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	m.ConfirmingDependencyChecks = true
	m.CheckChoiceYes = true

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Запустить проверки") {
		t.Fatalf("expected checks dialog in view, got:\n%s", view)
	}
}
