package model

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"charm.land/bubbles/v2/list"
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

	m := New(
		"",
		filepath.Join(home, ".config", "govm", "settings.json"),
		config.DefaultSettings(),
		"",
	)
	m.List.SetItems(items)
	m.Versions = []utils.GoVersion{{
		Version:   "1.24.4",
		Filename:  "go1.24.4.darwin-arm64.tar.gz",
		Installed: true,
		Active:    true,
		Path:      filepath.Join(home, ".govm", "versions", "go1.24.4"),
	}}
	m.Message = "Successfully installed Go 1.24.4"
	m.MessageType = "success"
	m.Loading = false
	m.Layout = styles.LayoutWide
	return m
}
