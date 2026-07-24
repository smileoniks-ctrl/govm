package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	coredeps "github.com/smileoniks-ctrl/govm/internal/deps"
)

func TestDepsTabArrowAndVimKeysMoveTableCursor(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Dependencies = []coredeps.ModuleDependency{
		{Path: "example.com/first", Version: "v1.0.0"},
		{Path: "example.com/second", Version: "v1.0.0"},
	}
	m.updateDependencyTable()
	m.Deps.Table.Focus()

	for _, tt := range []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "down", key: tea.KeyPressMsg{Code: tea.KeyDown}, want: 1},
		{name: "up", key: tea.KeyPressMsg{Code: tea.KeyUp}, want: 0},
		{name: "j", key: tea.KeyPressMsg{Code: 'j'}, want: 1},
		{name: "k", key: tea.KeyPressMsg{Code: 'k'}, want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			updated, _ := m.Update(tt.key)
			m = updated.(Model)

			if got := m.Deps.Table.Cursor(); got != tt.want {
				t.Fatalf("expected deps table cursor to move to index %d, got %d", tt.want, got)
			}
		})
	}
}

func TestDepsTabDownDoesNotMoveTableCursorWhileConfirmingUpdate(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Dependencies = []coredeps.ModuleDependency{
		{Path: "example.com/first", Version: "v1.0.0"},
		{Path: "example.com/second", Version: "v1.0.0"},
	}
	m.updateDependencyTable()
	m.Deps.Table.Focus()
	m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}

	before := m.Deps.Table.Cursor()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	if got := m.Deps.Table.Cursor(); got != before {
		t.Fatalf("expected deps table cursor to remain at index %d while update dialog is open, got %d", before, got)
	}
}

func TestDepsTabGlobalUOpensUpdateConfirmationForDirectUpdate(t *testing.T) {
	m := newTestModel(t)
	m.CurrentTab = DepsTab
	m.Deps.Loaded = true
	m.Deps.Dependencies = []coredeps.ModuleDependency{
		{Path: "example.com/direct", Version: "v1.0.0", Latest: "v1.1.0"},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'u'})
	m = updated.(Model)
	updated, _ = m.Update(coredeps.CheckUpdatesDoneEvent{Dependencies: m.Deps.Dependencies})
	m = updated.(Model)

	if m.Deps.Dialog.Kind != DialogUpdate {
		t.Fatal("expected fresh preflight to open the update confirmation")
	}
}
