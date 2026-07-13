package model

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
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
