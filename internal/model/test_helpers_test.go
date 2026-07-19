package model

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// assertVersionViewsConsistent verifies the postcondition that
// rebuildVersionViews is responsible for: the Available Versions list
// and the Installed Versions table must be exact projections of
// m.Versions. Used after msg-dispatch in update tests to catch any
// handler that mutates m.Versions without keeping the derived views in
// sync.
func assertVersionViewsConsistent(t *testing.T, m Model) {
	t.Helper()

	items := m.List.Items()
	if len(items) != len(m.Versions) {
		t.Fatalf("list length = %d, want %d (len m.Versions)", len(items), len(m.Versions))
	}

	wantInstalled := 0
	for i, v := range m.Versions {
		if v.Installed {
			wantInstalled++
		}
		it, ok := items[i].(styles.Item)
		if !ok {
			t.Fatalf("list item %d is %T, want styles.Item", i, items[i])
		}
		if it.Name != v.Version {
			t.Errorf("list item %d Name = %q, want %q", i, it.Name, v.Version)
		}
		if it.DescriptionText != v.DisplayDescription() {
			t.Errorf("list item %d DescriptionText = %q, want %q", i, it.DescriptionText, v.DisplayDescription())
		}
		if it.Installed != v.Installed {
			t.Errorf("list item %d Installed = %v, want %v", i, it.Installed, v.Installed)
		}
		if it.Active != v.Active {
			t.Errorf("list item %d Active = %v, want %v", i, it.Active, v.Active)
		}
	}

	rows := m.InstalledTable.Rows()
	if len(rows) != wantInstalled {
		t.Fatalf("installed table rows = %d, want %d", len(rows), wantInstalled)
	}

	rowIdx := 0
	for _, v := range m.Versions {
		if !v.Installed {
			continue
		}
		row := rows[rowIdx]
		rowIdx++
		if row[0] != v.Version {
			t.Errorf("installed row Version = %q, want %q", row[0], v.Version)
		}
		if row[1] != v.Path {
			t.Errorf("installed row Path = %q, want %q", row[1], v.Path)
		}
		wantStatus := ""
		if v.Active {
			wantStatus = "active"
		}
		if row[2] != wantStatus {
			t.Errorf("installed row Status = %q, want %q", row[2], wantStatus)
		}
	}
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

	m := New(
		"",
		filepath.Join(home, ".config", "govm", "settings.json"),
		config.DefaultSettings(),
		"",
	)
	m.Versions = []utils.GoVersion{{
		Version:   "1.24.4",
		Filename:  "go1.24.4.darwin-arm64.tar.gz",
		Installed: true,
		Active:    true,
		Path:      filepath.Join(home, ".govm", "versions", "go1.24.4"),
	}}
	m.rebuildVersionViews()
	m.Message = "Successfully installed Go 1.24.4"
	m.MessageType = "success"
	m.Loading = false
	m.Layout = styles.LayoutWide
	return m
}
