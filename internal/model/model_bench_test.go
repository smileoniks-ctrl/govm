package model

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// benchModel builds a Model that mimics a realistic TUI state for
// benchmarking the View/Update hot paths without network or filesystem
// side effects.
func benchModel(b *testing.B) Model {
	b.Helper()

	items := make([]list.Item, 0, 30)
	versions := make([]utils.GoVersion, 0, 30)
	for i := 0; i < 30; i++ {
		v := utils.GoVersion{
			Version:   "1.2" + string(rune('0'+i%10)),
			Filename:  "go1.2.darwin-arm64.tar.gz",
			Installed: i%3 == 0,
			Active:    i == 0,
		}
		if v.Installed {
			v.Path = "/Users/example/.govm/versions/go" + v.Version
		}
		versions = append(versions, v)
		items = append(items, styles.Item{
			Name:            v.Version,
			DescriptionText: "go" + v.Version + " " + v.Filename,
			Installed:       v.Installed,
			Active:          v.Active,
		})
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 24)
	l.SetShowHelp(false)

	installed := table.New(
		table.WithColumns(installedTableColumns(80, styles.LayoutNormal)),
		table.WithHeight(20),
	)
	deps := table.New(
		table.WithColumns(dependencyTableColumns(80, styles.LayoutNormal)),
		table.WithHeight(20),
	)

	depItems := make([]utils.ModuleDependency, 0, 10)
	for i := 0; i < 10; i++ {
		depItems = append(depItems, utils.ModuleDependency{
			Path:    "github.com/example/dep" + string(rune('a'+i)),
			Version: "v1.0.0",
			Latest:  "v1.1.0",
		})
	}

	depsState := NewDepsState("", deps)
	depsState.Dependencies = depItems
	depsState.Loaded = true

	return Model{
		List:           l,
		Versions:       versions,
		Spinner:        spinner.New(),
		InstalledTable: installed,
		Deps:           depsState,
		CurrentTab:     0,
		Layout:         styles.LayoutNormal,
		Width:          80,
		Height:         24,
	}
}

func BenchmarkView_NormalLayout(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkView_InstalledTab(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkView_DepsTab(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 2
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkView_DepsTabWithDialog(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 2
	m.Deps.Dialog.ConfirmingUpdate = true
	m.Deps.Dialog.UpdateChoiceYes = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkUpdate_WindowSizeMsg(b *testing.B) {
	m := benchModel(b)
	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(msg)
		_ = updated
	}
}

func BenchmarkUpdate_KeyPressMsg(b *testing.B) {
	m := benchModel(b)
	msg := tea.KeyPressMsg{Code: 'q'}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(msg)
		_ = updated
	}
}

func BenchmarkDependencyTableColumns(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dependencyTableColumns(120, styles.LayoutNormal)
	}
}

func BenchmarkInstalledTableColumns(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = installedTableColumns(120, styles.LayoutNormal)
	}
}

func BenchmarkOverlayDialog(b *testing.B) {
	bg := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	dlg := "AAA\nBBB\nCCC"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = overlayDialog(bg, dlg, 80, 24)
	}
}

func BenchmarkSpliceCentered(b *testing.B) {
	bg := "this is a moderately long background line for benchmarking splice behavior"
	overlay := "OVERLAY"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spliceCentered(bg, overlay, 12)
	}
}

func BenchmarkRenderDependencyUpdateDialog(b *testing.B) {
	entries := []utils.DependencyUpdateEntry{
		{Path: "github.com/example/dep1", OldVersion: "v1.0.0", NewVersion: "v1.1.0"},
		{Path: "github.com/example/dep2", OldVersion: "v2.0.0", NewVersion: "v2.1.0"},
		{Path: "github.com/example/dep3", OldVersion: "v3.0.0", NewVersion: "v3.1.0"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderDependencyUpdateDialog(true, entries)
		_ = renderDependencyUpdateDialog(false, entries)
	}
}
