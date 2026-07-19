package model

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

var (
	benchColumnsSink []table.Column
	benchModelSink   tea.Model
	benchStringSink  string
	benchViewSink    tea.View
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
			DescriptionText: v.DisplayDescription(),
			Installed:       v.Installed,
			Active:          v.Active,
		})
	}

	depItems := make([]utils.ModuleDependency, 0, 10)
	for i := 0; i < 10; i++ {
		depItems = append(depItems, utils.ModuleDependency{
			Path:    "github.com/example/dep" + string(rune('a'+i)),
			Version: "v1.0.0",
			Latest:  "v1.1.0",
		})
	}

	m := New("", "", config.DefaultSettings(), "")
	m.List.SetItems(items)
	m.List.SetSize(80, 24)
	m.Versions = versions
	m.Deps.Dependencies = depItems
	m.Deps.Loaded = true
	m.Loading = false
	m.Width = 80
	m.Height = 24
	return m
}

func benchModelAtViewport(b *testing.B, width int) Model {
	b.Helper()
	m := benchModel(b)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return updated.(Model)
}

func BenchmarkView_NormalLayout(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchViewSink = m.View()
	}
}

func BenchmarkView_InstalledTab(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 1
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchViewSink = m.View()
	}
}

func BenchmarkView_DepsTab(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 2
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchViewSink = m.View()
	}
}

func BenchmarkView_DepsTabWithDialog(b *testing.B) {
	m := benchModel(b)
	m.CurrentTab = 2
	m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchViewSink = m.View()
	}
}

func BenchmarkView_DepsTabWithPhysicalViewport(b *testing.B) {
	for _, width := range []int{64, 80, 120} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			m := benchModel(b)
			m.CurrentTab = DepsTab
			m.TermWidth = width
			m.TermHeight = 30
			m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchViewSink = m.View()
			}
		})
	}
}

func BenchmarkView_DepsTabWithPhysicalViewportWithoutDialog(b *testing.B) {
	for _, width := range []int{64, 80, 120} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			m := benchModelAtViewport(b, width)
			m.CurrentTab = DepsTab

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchViewSink = m.View()
			}
		})
	}
}

func BenchmarkView_DepsTabWithPhysicalViewportWithDialog(b *testing.B) {
	for _, width := range []int{64, 80, 120} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			m := benchModelAtViewport(b, width)
			m.CurrentTab = DepsTab
			m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchViewSink = m.View()
			}
		})
	}
}

func BenchmarkUpdate_WindowSizeMsg(b *testing.B) {
	m := benchModel(b)
	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(msg)
		benchModelSink = updated
	}
}

func BenchmarkUpdate_KeyPressMsg(b *testing.B) {
	m := benchModel(b)
	msg := tea.KeyPressMsg{Code: 'q'}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(msg)
		benchModelSink = updated
	}
}

func BenchmarkDependencyTableColumns(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchColumnsSink = dependencyTableColumns(120)
	}
}

func BenchmarkInstalledTableColumns(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchColumnsSink = installedTableColumns(120)
	}
}

func BenchmarkOverlayDialog(b *testing.B) {
	for _, size := range []viewportSize{
		{Width: 64, Height: 20},
		{Width: 80, Height: 30},
		{Width: 140, Height: 40},
	} {
		b.Run(strconv.Itoa(size.Width)+"x"+strconv.Itoa(size.Height), func(b *testing.B) {
			row := strings.Repeat("x", size.Width)
			bg := strings.TrimSuffix(strings.Repeat(row+"\n", size.Height), "\n")
			dlg := strings.Repeat("overlay dialog content\n", 4)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchStringSink = overlayDialog(bg, dlg, size)
			}
		})
	}
}

func BenchmarkSpliceCentered(b *testing.B) {
	bg := "this is a moderately long background line for benchmarking splice behavior"
	overlay := "OVERLAY"
	bgW := ansi.StringWidth(bg)
	overlayW := ansi.StringWidth(overlay)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchStringSink = spliceCentered(bg, overlay, 12, bgW, overlayW)
	}
}

func BenchmarkRenderContentCanvas(b *testing.B) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "plain", content: "first row\nsecond row\nthird row"},
		{name: "ANSI", content: "\x1b[31mfirst row\x1b[0m\n\x1b[32msecond row\x1b[0m\n\x1b[34mthird row\x1b[0m"},
		{name: "wide", content: "界e\u0301😀 first row\n界e\u0301😀 second row\n界e\u0301😀 third row"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchStringSink = renderContentCanvas(tc.content, 80, 24)
			}
		})
	}
}

func BenchmarkRenderDependencyUpdateDialog(b *testing.B) {
	entries := []utils.DependencyUpdateEntry{
		{Path: "github.com/example/dep1", OldVersion: "v1.0.0", NewVersion: "v1.1.0"},
		{Path: "github.com/example/dep2", OldVersion: "v2.0.0", NewVersion: "v2.1.0"},
		{Path: "github.com/example/dep3", OldVersion: "v3.0.0", NewVersion: "v3.1.0"},
	}
	updateYes := ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}
	updateNo := ConfirmDialog{Kind: DialogUpdate, ChoiceYes: false}
	deps := DepsState{UpdateEntries: entries}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchStringSink = updateYes.Render(deps, viewportSize{Width: 64, Height: 20})
		benchStringSink = updateNo.Render(deps, viewportSize{Width: 64, Height: 20})
	}
}

func BenchmarkRenderDependencyDialogsMinimumViewport(b *testing.B) {
	result := &utils.DependencyCheckResultMsg{
		Command: "go test ./...",
		Output:  "FAIL: dependency test output that is deliberately longer than the compact dialog content area",
	}
	backups := []utils.DependencyBackupInfo{{
		Name:    "2026-07-09_12-00-00-a-long-backup-filename.json",
		Kind:    utils.DependencyBackupKindPreUpdate,
		Updated: 1,
	}}
	checksDialog := ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
	rollbackDialog := ConfirmDialog{Kind: DialogRollback, ChoiceYes: true}
	restoreDialog := ConfirmDialog{Kind: DialogRestore, ChoiceYes: true}
	rollbackDeps := DepsState{LastCheckResult: result}
	restoreDeps := DepsState{Backups: backups}

	for _, width := range []int{64, 80} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				viewport := viewportSize{Width: width, Height: 20}
				benchStringSink = checksDialog.Render(DepsState{}, viewport)
				benchStringSink = rollbackDialog.Render(rollbackDeps, viewport)
				benchStringSink = restoreDialog.Render(restoreDeps, viewport)
			}
		})
	}
}
