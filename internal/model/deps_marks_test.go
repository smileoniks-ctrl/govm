package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

func marksFixture(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Loaded = true
	m.Deps.Dependencies = []deps.ModuleDependency{
		{Path: "example.com/a", Version: "v1.0.0", Latest: "v1.1.0"},
		{Path: "example.com/hidden", Version: "v0.1.0", Latest: "v0.2.0", Indirect: true},
		{Path: "example.com/b", Version: "v2.0.0", Latest: "v2.0.0"},
		{Path: "example.com/c", Version: "v3.0.0", Latest: "v3.1.0"},
	}
	m.updateDependencyTable()
	m.Deps.Table.Focus()
	return m
}

func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		updated, _ := m.Update(k)
		m = updated.(Model)
	}
	return m
}

func startedSelection(t *testing.T, m Model) deps.UpdateSelection {
	t.Helper()
	if m.Deps.Cycle.Phase() != deps.PhaseChecking {
		t.Fatalf("cycle phase = %s, want checking", m.Deps.Cycle.Phase())
	}
	return m.Deps.Cycle.Selection()
}

func TestSpaceTogglesMarkOnCursorRowAndRendersGlyph(t *testing.T) {
	m := marksFixture(t)
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.Deps.Marked("example.com/a") {
		t.Fatal("expected cursor row to be marked")
	}
	if got := m.Deps.Table.Rows()[0][0]; got != markFilled+"example.com/a" {
		t.Fatalf("row 0 = %q, want filled glyph prefix", got)
	}
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.Deps.Marked("example.com/a") {
		t.Fatal("expected second space to clear the mark")
	}
	if got := m.Deps.Table.Rows()[0][0]; got != markEmpty+"example.com/a" {
		t.Fatalf("row 0 = %q, want empty glyph prefix", got)
	}
}

func TestMarkOnRowWithoutUpdateStillRendersFilledGlyph(t *testing.T) {
	m := marksFixture(t)
	// Hidden indirect row is skipped: row 1 is example.com/b.
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.Deps.Marked("example.com/b") {
		t.Fatalf("expected example.com/b marked, marks = %v", m.Deps.Marks)
	}
	if got := m.Deps.Table.Rows()[1][0]; !strings.HasPrefix(got, markFilled) {
		t.Fatalf("row 1 = %q, want filled glyph", got)
	}
	// Unmarked rows keep the empty glyph so the column reads as a checklist.
	if got := m.Deps.Table.Rows()[0][0]; !strings.HasPrefix(got, markEmpty) {
		t.Fatalf("row 0 = %q, want empty glyph", got)
	}
}

func TestUpdateScopeIsAllDirectUnlessMarked(t *testing.T) {
	m := marksFixture(t)
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }

	// No marks: every direct dependency, regardless of the cursor.
	got := startedSelection(t, press(t, m, tea.KeyPressMsg{Code: 'j'}, tea.KeyPressMsg{Code: 'u'}))
	if got.Explicit() {
		t.Fatalf("selection = %#v, want all direct dependencies", got)
	}

	// Marks win over the cursor and keep dependency order.
	m2 := press(t, m,
		tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeySpace}, // c
		tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeySpace}, // a
		tea.KeyPressMsg{Code: tea.KeyDown}, // cursor on b, which is not marked
		tea.KeyPressMsg{Code: 'u'},
	)
	got = startedSelection(t, m2)
	if len(got.Modules) != 2 || got.Modules[0] != "example.com/a" || got.Modules[1] != "example.com/c" {
		t.Fatalf("selection = %#v, want [a c]", got.Modules)
	}
	if got.Level != deps.LevelLatest {
		t.Fatalf("level = %s, want latest", got.Level)
	}
}

func TestUpdateOnEmptyTableReportsNothingToUpdate(t *testing.T) {
	m := marksFixture(t)
	m.Deps.Dependencies = nil
	m.updateDependencyTable()
	m = press(t, m, tea.KeyPressMsg{Code: 'u'})
	if m.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("cycle started with nothing under the cursor: %s", m.Deps.Cycle.Phase())
	}
	if !strings.Contains(m.Status.Text(), "Nothing to update") {
		t.Fatalf("status = %q", m.Status.Text())
	}
}

func TestMarkAllMarksEveryListedRowInBothDisplayModes(t *testing.T) {
	wants := map[config.DepsDisplayMode][]string{
		config.DepsDisplayDirect: {"example.com/a", "example.com/b", "example.com/c"},
		config.DepsDisplayAll:    {"example.com/a", "example.com/hidden", "example.com/b", "example.com/c"},
	}
	for display, want := range wants {
		m := marksFixture(t)
		m.Settings.Values.DepsDisplay = display
		m.updateDependencyTable()
		m = press(t, m, tea.KeyPressMsg{Code: 'a'})
		if got := m.Deps.MarkedPaths(); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("display %v: marks = %v, want %v", display, got, want)
		}
		for _, row := range m.Deps.Table.Rows() {
			if !strings.HasPrefix(row[0], markFilled) {
				t.Fatalf("display %v: row %q not rendered as marked", display, row[0])
			}
		}
		// Second press clears everything.
		m = press(t, m, tea.KeyPressMsg{Code: 'a'})
		if len(m.Deps.Marks) != 0 {
			t.Fatalf("display %v: expected marks cleared, got %v", display, m.Deps.Marks)
		}
	}
}

func TestMarkAllAfterManualMarkClearsEverything(t *testing.T) {
	m := marksFixture(t)
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace}, tea.KeyPressMsg{Code: 'a'})
	if len(m.Deps.Marks) != 0 {
		t.Fatalf("marks = %v, want none: a with any mark present clears all", m.Deps.Marks)
	}
	if !strings.Contains(m.Status.Text(), "Marks cleared") {
		t.Fatalf("status = %q", m.Status.Text())
	}
}

func TestMarksSurviveRefreshAndDisplayToggle(t *testing.T) {
	m := marksFixture(t)
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	updated, _ := m.Update(DependenciesMsg(m.Deps.Dependencies))
	m = updated.(Model)
	if !m.Deps.Marked("example.com/a") {
		t.Fatal("mark lost after DependenciesMsg")
	}
	m.Settings.Values.DepsDisplay = config.DepsDisplayAll
	m.updateDependencyTable()
	if !m.Deps.Marked("example.com/a") || !strings.HasPrefix(m.Deps.Table.Rows()[0][0], markFilled) {
		t.Fatal("mark lost after display toggle")
	}
	// A module that vanished from the list loses its mark.
	m.Deps.Dependencies = m.Deps.Dependencies[1:]
	m.updateDependencyTable()
	if m.Deps.Marked("example.com/a") {
		t.Fatal("stale mark should be pruned")
	}
}

func TestMarksClearedAfterCycleEndsButKeptOnCancel(t *testing.T) {
	m := marksFixture(t)
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace}, tea.KeyPressMsg{Code: 'u'})
	updated, _ := m.Update(deps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)
	if m.Deps.Dialog.Kind != DialogUpdate || !m.Deps.Dialog.Explicit {
		t.Fatalf("dialog = %#v, want explicit update dialog", m.Deps.Dialog)
	}
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.Deps.Marked("example.com/a") {
		t.Fatal("cancel must keep marks")
	}

	// Run to a terminal outcome: no updates for the marked module.
	m.Deps.Dependencies[0].Latest = "v1.0.0"
	m = press(t, m, tea.KeyPressMsg{Code: 'u'})
	updated, _ = m.Update(deps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)
	if m.Deps.Cycle.Phase() != deps.PhaseIdle || len(m.Deps.Marks) != 0 {
		t.Fatalf("phase = %s marks = %v, want idle and no marks", m.Deps.Cycle.Phase(), m.Deps.Marks)
	}
	if !strings.Contains(m.Status.Text(), "Already up to date: example.com/a") {
		t.Fatalf("status = %q", m.Status.Text())
	}
}

// openUpdateDialog presses u and completes the check so the update
// dialog is open.
func openUpdateDialog(t *testing.T, m Model) Model {
	t.Helper()
	m = press(t, m, tea.KeyPressMsg{Code: 'u'})
	updated, _ := m.Update(deps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)
	if m.Deps.Dialog.Kind != DialogUpdate {
		t.Fatalf("dialog = %#v, want update dialog", m.Deps.Dialog)
	}
	return m
}

func entryPaths(entries []deps.DependencyUpdateEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	return paths
}

func TestDialogSpaceTogglesScopeBetweenAllAndCursorModule(t *testing.T) {
	m := marksFixture(t)
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }
	// Cursor on example.com/b (current); nothing marked.
	m = openUpdateDialog(t, press(t, m, tea.KeyPressMsg{Code: 'j'}))
	if m.Deps.Dialog.Explicit {
		t.Fatal("scope should start at all direct dependencies")
	}
	if got := entryPaths(m.Deps.Dialog.UpdateEntries); strings.Join(got, ",") != "example.com/a,example.com/c" {
		t.Fatalf("all-scope entries = %v", got)
	}
	if got := m.Deps.Dialog.ExplicitModules; len(got) != 1 || got[0] != "example.com/b" {
		t.Fatalf("explicit modules = %v, want cursor module", got)
	}
	rendered := stripANSI(m.Deps.Dialog.Render(testTheme(), m.Deps, viewportSize{Width: 64, Height: 24}))
	for _, want := range []string{"Scope:", "All", "Current"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q:\n%s", want, rendered)
		}
	}

	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.Deps.Dialog.Explicit || m.Deps.Dialog.Kind != DialogUpdate {
		t.Fatalf("dialog after space = %#v, want explicit scope", m.Deps.Dialog)
	}
	if sel := m.Deps.Cycle.Selection(); len(sel.Modules) != 1 || sel.Modules[0] != "example.com/b" {
		t.Fatalf("cycle selection = %#v, want cursor module", sel)
	}
	if len(m.Deps.Dialog.UpdateEntries) != 0 {
		t.Fatalf("expected empty plan for current module, got %v", m.Deps.Dialog.UpdateEntries)
	}

	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.Deps.Dialog.Explicit || m.Deps.Cycle.Selection().Explicit() {
		t.Fatal("second space should return to all direct dependencies")
	}
	if got := entryPaths(m.Deps.Dialog.UpdateEntries); strings.Join(got, ",") != "example.com/a,example.com/c" {
		t.Fatalf("entries after toggling back = %v", got)
	}
}

func TestDialogScopeStartsMarkedAndKeepsLevelAcrossToggle(t *testing.T) {
	m := marksFixture(t)
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }
	m = openUpdateDialog(t, press(t, m, tea.KeyPressMsg{Code: tea.KeySpace}))
	if !m.Deps.Dialog.Explicit {
		t.Fatal("scope should start at the marked set")
	}
	if got := entryPaths(m.Deps.Dialog.UpdateEntries); strings.Join(got, ",") != "example.com/a" {
		t.Fatalf("marked-scope entries = %v", got)
	}
	rendered := stripANSI(m.Deps.Dialog.Render(testTheme(), m.Deps, viewportSize{Width: 64, Height: 24}))
	if !strings.Contains(rendered, "Marked (1)") {
		t.Fatalf("dialog missing marked label:\n%s", rendered)
	}

	// Level change survives a scope toggle in both directions.
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.Deps.Dialog.Explicit {
		t.Fatal("space should switch to all direct dependencies")
	}
	if got := entryPaths(m.Deps.Dialog.UpdateEntries); strings.Join(got, ",") != "example.com/a,example.com/c" {
		t.Fatalf("all-scope entries = %v", got)
	}
	if sel := m.Deps.Cycle.Selection(); sel.Level != deps.LevelMinor || sel.Explicit() {
		t.Fatalf("cycle selection = %#v, want minor level over all direct", sel)
	}
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if sel := m.Deps.Cycle.Selection(); sel.Level != deps.LevelMinor || len(sel.Modules) != 1 || sel.Modules[0] != "example.com/a" {
		t.Fatalf("cycle selection = %#v, want minor level over marked", sel)
	}
	// Marks are untouched by scope toggling.
	if !m.Deps.Marked("example.com/a") || len(m.Deps.Marks) != 1 {
		t.Fatalf("marks = %v, want only a", m.Deps.Marks)
	}
}

func TestMarkKeysInertWhileOperationInProgress(t *testing.T) {
	m := marksFixture(t)
	m.Deps.Phase = OpChecking
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeySpace}, tea.KeyPressMsg{Code: 'a'})
	if len(m.Deps.Marks) != 0 {
		t.Fatalf("marks changed during operation: %v", m.Deps.Marks)
	}
}

func TestUpdateDialogArrowsCycleLevelAndRebuildPlan(t *testing.T) {
	m := marksFixture(t)
	m.Deps.Dependencies[0].Versions = []string{"v1.0.0", "v1.0.5", "v1.1.0"}
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }
	m = press(t, m, tea.KeyPressMsg{Code: 'u'})
	updated, _ := m.Update(deps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)
	if m.Deps.Dialog.Level != deps.LevelLatest || m.Deps.Dialog.UpdateEntries[0].NewVersion != "v1.1.0" {
		t.Fatalf("initial dialog = %#v", m.Deps.Dialog)
	}
	// Highlight "No", then change level: the highlight must survive.
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Deps.Dialog.Level != deps.LevelMinor || m.Deps.Dialog.ChoiceYes {
		t.Fatalf("after up: level = %s choiceYes = %v", m.Deps.Dialog.Level, m.Deps.Dialog.ChoiceYes)
	}
	m = press(t, m, tea.KeyPressMsg{Code: 'k'})
	if m.Deps.Dialog.Level != deps.LevelPatch || m.Deps.Dialog.UpdateEntries[0].NewVersion != "v1.0.5" {
		t.Fatalf("after k: dialog = %#v", m.Deps.Dialog)
	}
	if m.Deps.Cycle.Selection().Level != deps.LevelPatch {
		t.Fatalf("cycle level = %s, want patch", m.Deps.Cycle.Selection().Level)
	}
	// Wrap around: patch -> up -> latest.
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Deps.Dialog.Level != deps.LevelLatest {
		t.Fatalf("expected wrap to latest, got %s", m.Deps.Dialog.Level)
	}
	view := stripANSI(m.Deps.Dialog.Render(testTheme(), m.Deps, viewportSize{Width: 80, Height: 24}))
	if !strings.Contains(view, "Level:") || !strings.Contains(view, "v1.1.0") {
		t.Fatalf("dialog view missing level line or entry:\n%s", view)
	}
}

func TestUpdateDialogEmptyPlanConfirmEndsAsNoUpdates(t *testing.T) {
	m := marksFixture(t)
	m.Deps.Dependencies[0].Versions = []string{"v1.0.0", "v1.1.0"}
	m.Deps.ExecuteIntent = func(deps.Intent) tea.Cmd { return nil }
	m = press(t, m, tea.KeyPressMsg{Code: 'u'})
	updated, _ := m.Update(deps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyDown}) // latest -> patch (wrap)
	if m.Deps.Dialog.Level != deps.LevelPatch || len(m.Deps.Dialog.UpdateEntries) != 0 {
		t.Fatalf("dialog = %#v, want empty patch plan", m.Deps.Dialog)
	}
	view := stripANSI(m.Deps.Dialog.Render(testTheme(), m.Deps, viewportSize{Width: 80, Height: 24}))
	if !strings.Contains(view, "No updates available at the patch level") {
		t.Fatalf("dialog view:\n%s", view)
	}
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Deps.Dialog.Active() || m.Deps.Cycle.Phase() != deps.PhaseIdle {
		t.Fatalf("dialog active = %v phase = %s", m.Deps.Dialog.Active(), m.Deps.Cycle.Phase())
	}
	if !strings.Contains(m.Status.Text(), "at the patch level") {
		t.Fatalf("status = %q", m.Status.Text())
	}
}

func TestDepsHintBarAndOverlayDocumentMarkKeys(t *testing.T) {
	bar := stripANSI(renderHelpBar(testTheme(), Model{CurrentTab: DepsTab}, 120))
	for _, want := range []string{"space mark", "a mark all / none", "u update"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("hint bar missing %q: %s", want, bar)
		}
	}
	if !hasBinding(tabKeyBindings(DepsTab), "a") {
		t.Fatal("Deps section must document the a key")
	}
	if !hasBinding(dialogKeyBindings(ConfirmDialog{Kind: DialogUpdate}), "↑/↓ k/j") {
		t.Fatal("update dialog section must document the level keys")
	}
	// The scope key is offered only when there is an explicit set to switch to.
	if hasBinding(dialogKeyBindings(ConfirmDialog{Kind: DialogUpdate}), "space") {
		t.Fatal("update dialog without an explicit set must not offer the scope key")
	}
	withScope := ConfirmDialog{Kind: DialogUpdate, ExplicitModules: []string{"example.com/a"}}
	if !hasBinding(dialogKeyBindings(withScope), "space") {
		t.Fatal("update dialog with an explicit set must document the scope key")
	}
	dialogBar := stripANSI(renderHelpBar(testTheme(), Model{CurrentTab: DepsTab, Deps: DepsState{Dialog: withScope}}, 120))
	if !strings.Contains(dialogBar, "space scope") {
		t.Fatalf("dialog hint bar missing scope key: %s", dialogBar)
	}
}
