package model

import (
	"encoding/json"
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
	"github.com/smileoniks-ctrl/govm/internal/config"
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
	depTbl := table.New(
		table.WithColumns(dependencyTableColumns(80, styles.LayoutNormal)),
		table.WithHeight(20),
	)

	return Model{
		List:           l,
		Versions:       []utils.GoVersion{{Version: "1.24.4", Filename: "go1.24.4.darwin-arm64.tar.gz", Installed: true, Active: true, Path: filepath.Join(home, ".govm", "versions", "go1.24.4")}},
		Spinner:        spinner.New(),
		HomeDir:        home,
		GoVersionsDir:  filepath.Join(home, ".govm", "versions"),
		InstalledTable: tbl,
		Deps:           NewDepsState("", depTbl),
		Settings:       NewSettingsState(filepath.Join(home, ".config", "govm", "settings.json"), config.DefaultSettings()),
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
	m.updateInstalledTable()
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

func TestSettingsTabRendersRowsAndHelp(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = SettingsTab

	view := stripANSI(m.View().Content)

	for _, want := range []string{
		"Settings",
		"Deps display: Direct only",
		"Theme: Current",
		"Deps backups: 10",
		"↑/↓",
		"enter",
		"tab",
		"q",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected settings view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestSettingsDepsBackupLimitControlsAndSaves(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		start int
		want  int
	}{
		{name: "left wraps minimum to maximum", key: tea.KeyPressMsg{Code: tea.KeyLeft}, start: 1, want: 100},
		{name: "right wraps maximum to minimum", key: tea.KeyPressMsg{Code: tea.KeyRight}, start: 100, want: 1},
		{name: "h wraps minimum to maximum", key: tea.KeyPressMsg{Code: 'h'}, start: 1, want: 100},
		{name: "l wraps maximum to minimum", key: tea.KeyPressMsg{Code: 'l'}, start: 100, want: 1},
		{name: "space wraps maximum to minimum", key: tea.KeyPressMsg{Code: ' '}, start: 100, want: 1},
		{name: "enter increments limit", key: tea.KeyPressMsg{Code: tea.KeyEnter}, start: 1, want: 2},
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
	currentDialog := renderDependencyChecksDialog(true)

	if got := ApplyTheme(styles.ThemeLight); got != styles.ThemeLight {
		t.Fatalf("expected light theme, got %q", got)
	}

	lightDialog := renderDependencyChecksDialog(true)
	if lightDialog == currentDialog {
		t.Fatal("expected light theme to change dependency dialog output")
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

	if got.Deps.Table.Width() <= 0 || got.Deps.Table.Height() <= 0 {
		t.Fatalf("expected positive deps table size, got %dx%d", got.Deps.Table.Width(), got.Deps.Table.Height())
	}
}

func TestWindowSizeMsgCompactUsesPhysicalContentWidth(t *testing.T) {
	tests := []struct {
		name      string
		termWidth int
		wantWidth int
	}{
		{name: "minimum terminal", termWidth: 30, wantWidth: 28},
		{name: "below compact breakpoint", termWidth: 59, wantWidth: 57},
		{name: "compact breakpoint", termWidth: 60, wantWidth: 56},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: tt.termWidth, Height: 24})
			got := updated.(Model)

			if got.Width != tt.wantWidth {
				t.Fatalf("content width = %d, want %d", got.Width, tt.wantWidth)
			}
			if got.Deps.Table.Width() != tt.wantWidth {
				t.Fatalf("deps table width = %d, want %d", got.Deps.Table.Width(), tt.wantWidth)
			}
		})
	}
}

func TestTruncateViewWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		check func(t *testing.T, got string)
	}{
		{
			name:  "ANSI-styled line that overflows retains its reset sequence and width <= max",
			input: "\x1b[31mabcdef\x1b[0m",
			width: 3,
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "\x1b[0m") {
					t.Fatalf("expected ANSI reset sequence in %q", got)
				}
				if gotWidth := ansi.StringWidth(got); gotWidth > 3 {
					t.Fatalf("line width = %d, want <= 3: %q", gotWidth, got)
				}
			},
		},
		{
			name:  "multiple lines preserves normal lines",
			input: "first\nthis line overflows\nlast",
			width: 6,
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.Contains(got, "first") || !strings.Contains(got, "last") {
					t.Fatalf("expected normal lines preserved, got %q", got)
				}
			},
		},
		{
			name:  "trailing empty line/newline preserved",
			input: "this line overflows\n",
			width: 4,
			check: func(t *testing.T, got string) {
				t.Helper()
				if !strings.HasSuffix(got, "\n") {
					t.Fatalf("expected trailing newline preserved, got %q", got)
				}
				if gotLines := strings.Split(got, "\n"); len(gotLines) != 2 || gotLines[1] != "" {
					t.Fatalf("expected trailing empty line preserved, got %q", got)
				}
			},
		},
		{
			name:  "no-overflow returns identical content",
			input: "\x1b[32mshort\x1b[0m\nsecond",
			width: 10,
			check: func(t *testing.T, got string) {
				t.Helper()
				if got != "\x1b[32mshort\x1b[0m\nsecond" {
					t.Fatalf("got %q, want identical input", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateViewWidth(tt.input, tt.width)
			tt.check(t, got)
		})
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

	if !m.Deps.Checking {
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

	if !m.Deps.LoadingBackups {
		t.Fatal("expected LoadingBackups to be true after pressing b on deps tab")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
}

func TestDependencyBackupsMsgOpensRestoreDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(utils.DependencyBackupsMsg{
		{
			Name:       "2026-07-09_12-00-00.json",
			ModulePath: "github.com/acme/app",
			Kind:       utils.DependencyBackupKindPreUpdate,
			Updated:    1,
		},
	})
	got := updated.(Model)

	if got.Deps.LoadingBackups {
		t.Fatal("expected LoadingBackups to be false after backups load")
	}
	if !got.Deps.Dialog.ConfirmingRestoreBackup {
		t.Fatal("expected restore dialog to open")
	}
	if got.Deps.BackupCursor != 0 {
		t.Fatalf("expected backup cursor 0, got %d", got.Deps.BackupCursor)
	}
}

func TestRestoreBackupDialogEnterTriggersRestoreCmd(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRestoreBackup = true
	m.Deps.Dialog.RestoreChoiceYes = true
	m.Deps.Backups = []utils.DependencyBackupInfo{
		{Name: "2026-07-09_12-00-00.json"},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRestoreBackup {
		t.Fatal("expected restore dialog to close")
	}
	if !got.Deps.RestoringBackup {
		t.Fatal("expected RestoringBackup to be true")
	}
	if cmd == nil {
		t.Fatal("expected restore command")
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

	if !got.Deps.Loaded {
		t.Fatal("expected DependenciesLoaded to be true")
	}

	if len(got.Deps.Dependencies) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(got.Deps.Dependencies))
	}

	rows := got.Deps.Table.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 direct rows in table, got %d", len(rows))
	}

	if rows[0][3] != "update avail" {
		t.Fatalf("expected 'update avail' status, got %q", rows[0][3])
	}
	if rows[1][3] != "current" {
		t.Fatalf("expected 'current' status, got %q", rows[1][3])
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

	if !m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected ConfirmingDependencyUpdate to be true after pressing u on deps tab")
	}
	if !m.Deps.Dialog.UpdateChoiceYes {
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

	if m.Deps.Dialog.ConfirmingUpdate {
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

	if m.Deps.Dialog.ConfirmingUpdate {
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

	if !m.Deps.Dialog.UpdateChoiceYes {
		t.Fatal("expected default to be Yes")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(Model)

	if m.Deps.Dialog.UpdateChoiceYes {
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

	if m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected dialog to close after confirm on No")
	}
	if m.Deps.Updating {
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

	if m.Deps.Dialog.ConfirmingUpdate {
		t.Fatal("expected dialog to close after confirm on Yes")
	}
	if !m.Deps.Updating {
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

	if got.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be false after update complete")
	}
	if len(got.Deps.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Deps.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got type %q", got.MessageType)
	}
}

func TestDependencyTableIndirectUpdateStatus(t *testing.T) {
	m := newTestModel(t)
	m.Settings.Values.DepsDisplay = config.DepsDisplayAll

	deps := utils.DependenciesMsg{
		{Path: "indirect-with-update", Version: "v0.5.0", Latest: "v0.6.0", Indirect: true},
	}

	updated, _ := m.Update(deps)
	got := updated.(Model)

	rows := got.Deps.Table.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][3] != "indirect update" {
		t.Fatalf("expected 'indirect update' status, got %q", rows[0][3])
	}
}

func TestRenderDependencyUpdateDialogContainsWarning(t *testing.T) {
	dialog := stripANSI(renderDependencyUpdateDialog(true, nil))

	for _, want := range []string{"Warning", "will be updated", "Yes", "No"} {
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

	if !strings.Contains(dialog, "and") || !strings.Contains(dialog, "more") {
		t.Fatalf("expected truncation hint in dialog, got:\n%s", dialog)
	}
}

func TestRenderDependencyRestoreDialogKeepsCursorVisible(t *testing.T) {
	backups := make([]utils.DependencyBackupInfo, 0, 7)
	for i := 0; i < 7; i++ {
		backups = append(backups, utils.DependencyBackupInfo{
			Name:    fmt.Sprintf("2026-07-09_12-00-0%d.json", i),
			Kind:    utils.DependencyBackupKindPreUpdate,
			Updated: i,
		})
	}

	dialog := stripANSI(renderDependencyRestoreDialog(true, backups, 6))

	if !strings.Contains(dialog, "> 2026-07-09_12-00-06.json") {
		t.Fatalf("expected selected backup to be visible, got:\n%s", dialog)
	}
}

func TestRenderDependencyChecksDialogContainsCommands(t *testing.T) {
	dialog := stripANSI(renderDependencyChecksDialog(true))

	for _, want := range []string{"Run checks?", "go test", "go vet", "Yes", "No"} {
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

	for _, want := range []string{"Checks failed", "go test ./...", "FAIL: example_test", "Roll back", "Keep"} {
		if !strings.Contains(dialog, want) {
			t.Fatalf("expected dialog to contain %q, got:\n%s", want, dialog)
		}
	}
}

func TestRenderDependencyDialogsRespectViewportWidth(t *testing.T) {
	longPath := "github.com/acme/very-long-module-name-that-must-not-overflow-the-terminal"
	longOutput := "FAIL: " + strings.Repeat("a", 100)
	backups := []utils.DependencyBackupInfo{{
		Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
		Kind:    utils.DependencyBackupKindPreUpdate,
		Updated: 1,
	}}

	tests := []struct {
		name   string
		render func(width int) string
	}{
		{
			name: "update",
			render: func(width int) string {
				return renderDependencyUpdateDialog(true, []utils.DependencyUpdateEntry{{
					Path: longPath, OldVersion: "v1.0.0", NewVersion: "v1.1.0",
				}}, width)
			},
		},
		{
			name: "checks",
			render: func(width int) string {
				return renderDependencyChecksDialog(true, width)
			},
		},
		{
			name: "rollback",
			render: func(width int) string {
				return renderDependencyRollbackDialog(true, &utils.DependencyCheckResultMsg{
					Command: longPath,
					Output:  longOutput,
				}, width)
			},
		},
		{
			name: "restore",
			render: func(width int) string {
				return renderDependencyRestoreDialog(true, backups, 0, width)
			},
		},
	}

	for _, width := range []int{30, 59, 60} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s-%d", tt.name, width), func(t *testing.T) {
				for _, line := range strings.Split(tt.render(width), "\n") {
					if got := ansi.StringWidth(line); got > width {
						t.Fatalf("line width = %d, want <= %d: %q", got, width, line)
					}
				}
			})
		}
	}
}

func TestViewDependencyDialogsRespectTerminalWidth(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Model)
	}{
		{
			name: "update",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingUpdate = true
				m.Deps.Dependencies = []utils.ModuleDependency{{
					Path:    "github.com/acme/very-long-module-name-that-must-not-overflow-the-terminal",
					Version: "v1.0.0",
					Latest:  "v1.1.0",
				}}
			},
		},
		{
			name: "checks",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingChecks = true
			},
		},
		{
			name: "rollback",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingRollback = true
				m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{
					Command: "go test ./...",
					Output:  strings.Repeat("failure output ", 12),
				}
			},
		},
		{
			name: "restore",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingRestoreBackup = true
				m.Deps.Backups = []utils.DependencyBackupInfo{{
					Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
					Kind:    utils.DependencyBackupKindPreUpdate,
					Updated: 1,
				}}
			},
		},
	}

	for _, width := range []int{30, 59, 60} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s-%d", tt.name, width), func(t *testing.T) {
				m := newTestModel(t)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
				m = updated.(Model)
				tt.setup(&m)

				for _, line := range strings.Split(m.View().Content, "\n") {
					if got := ansi.StringWidth(line); got > width {
						t.Fatalf("view line width = %d, want <= %d: %q", got, width, line)
					}
				}
			})
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
	m.Settings.Values.DepsDisplay = config.DepsDisplayAll

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
	rows := got.Deps.Table.Rows()
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
	got := renderHelp(0, true, false, false, false, false, false, 80, styles.LayoutNormal)
	if !strings.Contains(stripANSI(got), "confirm") {
		t.Fatalf("expected confirm hint, got: %s", got)
	}
	if !strings.Contains(stripANSI(got), "cancel") {
		t.Fatalf("expected cancel hint, got: %s", got)
	}
}

func TestRenderHelp_RestoreUsesSelectedAction(t *testing.T) {
	tests := []struct {
		name             string
		restoreChoiceYes bool
		want             string
	}{
		{name: "restore selected", restoreChoiceYes: true, want: "enter restore"},
		{name: "cancel selected", restoreChoiceYes: false, want: "enter cancel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(renderHelp(DepsTab, false, false, false, false, true, tt.restoreChoiceYes, 80, styles.LayoutNormal))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("expected help to contain %q, got %q", tt.want, got)
			}
		})
	}
}

func TestRenderHelp_DepsCompactTruncates(t *testing.T) {
	got := renderHelp(2, false, false, false, false, false, false, 20, styles.LayoutCompact)
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

	if got.Deps.Updating {
		t.Fatal("expected UpdatingDependencies to be false")
	}
	if got.Deps.Snapshot == nil {
		t.Fatal("expected LastDependencySnapshot to be set")
	}
	if !got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to be true")
	}
	if !got.Deps.Dialog.CheckChoiceYes {
		t.Fatal("expected CheckChoiceYes default to be Yes")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success message, got %q", got.MessageType)
	}
}

func TestDependencyCheckResultOKClearsDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}

	updated, _ := m.Update(utils.DependencyCheckResultMsg{OK: true})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close after success")
	}
	if got.Deps.RunningChecks {
		t.Fatal("expected RunningDependencyChecks to be false")
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
	if got.Deps.Snapshot != nil {
		t.Fatal("expected Snapshot to be cleared after success")
	}
	if got.Deps.LastCheckResult != nil {
		t.Fatal("expected LastCheckResult to be cleared after success")
	}
}

func TestDependencyCheckResultFailOpensRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true
	m.Deps.RunningChecks = true

	msg := utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL",
	}

	updated, _ := m.Update(msg)
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected ConfirmingDependencyChecks to close on failure")
	}
	if !got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to be true")
	}
	if !got.Deps.Dialog.RollbackChoiceYes {
		t.Fatal("expected RollbackChoiceYes default to be Yes")
	}
	if got.MessageType != "error" {
		t.Fatalf("expected error status, got %q", got.MessageType)
	}
	if got.Deps.LastCheckResult == nil || got.Deps.LastCheckResult.Command != "go test ./..." {
		t.Fatalf("expected LastCheckResult to capture failing command, got %+v", got.Deps.LastCheckResult)
	}
}

func TestRollbackCmdTriggeredByRollbackYes(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{
		ModFile: utils.ModuleFileSnapshot{Exists: true, Content: "old"},
		SumFile: utils.ModuleFileSnapshot{Exists: true, Content: "oldsum"},
	}
	m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test ./..."}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close")
	}
	if !got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to be true")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned for rollback")
	}
}

func TestKeepCmdClearsRollbackDialog(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}
	m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{OK: false, Command: "go test"}

	// Toggle to No then confirm.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected ConfirmingDependencyRollback to close when keeping updates")
	}
	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
	if got.MessageType != "warning" {
		t.Fatalf("expected warning status, got %q", got.MessageType)
	}
}

func TestEscOnChecksDialogSkipsChecks(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingChecks {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.RunningChecks {
		t.Fatal("expected RunningDependencyChecks to remain false")
	}
}

func TestEscOnRollbackDialogKeepsUpdates(t *testing.T) {
	m := newTestModel(t)
	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(Model)

	if got.Deps.Dialog.ConfirmingRollback {
		t.Fatal("expected dialog to close on esc")
	}
	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to remain false")
	}
}

func TestDependenciesRolledBackMsgUpdatesState(t *testing.T) {
	m := newTestModel(t)
	m.Deps.RollingBack = true
	m.Deps.Snapshot = &utils.DependencySnapshot{}

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

	if got.Deps.RollingBack {
		t.Fatal("expected RollingBackDependencies to be false")
	}
	if len(got.Deps.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(got.Deps.Dependencies))
	}
	if got.MessageType != "success" {
		t.Fatalf("expected success status, got %q", got.MessageType)
	}
}

func TestDependencyErrDuringRollbackClearsState(t *testing.T) {
	m := newTestModel(t)
	m.Deps.RollingBack = true

	updated, _ := m.Update(utils.DependencyErrMsg{Err: errors.New("boom")})
	got := updated.(Model)

	if got.Deps.RollingBack {
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

	m.Deps.Dialog.ConfirmingRollback = true
	m.Deps.Dialog.RollbackChoiceYes = true
	m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{
		OK:      false,
		Command: "go test ./...",
		Output:  "FAIL: foo_test.go:42",
	}

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Checks failed") {
		t.Fatalf("expected rollback dialog in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Roll back") {
		t.Fatal("expected rollback button in view")
	}
}

func TestViewShowsChecksDialog(t *testing.T) {
	m := newTestModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '\t'})
	m = updated.(Model)
	m.Deps.Dialog.ConfirmingChecks = true
	m.Deps.Dialog.CheckChoiceYes = true

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Run checks?") {
		t.Fatalf("expected checks dialog in view, got:\n%s", view)
	}
}
