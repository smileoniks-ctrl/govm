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
