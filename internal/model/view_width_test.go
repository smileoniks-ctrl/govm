package model

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

func TestViewRespectsTerminalWidth(t *testing.T) {
	dialogs := []struct {
		name  string
		setup func(*Model)
	}{
		{
			name:  "without dialog",
			setup: func(*Model) {},
		},
		{
			name: "update dialog",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}
			},
		},
		{
			name: "checks dialog",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
			},
		},
		{
			name: "rollback dialog",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{
					Kind:      DialogRollback,
					ChoiceYes: true,
					CheckResult: &deps.DependencyCheckResult{
						Command: "go test ./...",
						Output:  strings.Repeat("failure output ", 12),
					},
				}
			},
		},
		{
			name: "restore dialog",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogRestore, ChoiceYes: true}
				m.Deps.Backups = []deps.DependencyBackupInfo{{
					Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
					Kind:    deps.DependencyBackupKindPreUpdate,
					Updated: 1,
				}}
			},
		},
		{
			name: "backup limit dialog",
			setup: func(m *Model) {
				m.CurrentTab = SettingsTab
				m.Settings.Cursor = 2
				m.Settings.OpenDepsBackupLimitInput()
			},
		},
	}

	for _, width := range []int{64, 80, 120} {
		for tab := AvailableTab; tab <= SettingsTab; tab++ {
			for _, dialog := range dialogs {
				t.Run(strconv.Itoa(width)+"/tab-"+strconv.Itoa(tab)+"/"+dialog.name, func(t *testing.T) {
					m := newTestModel(t)
					updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
					m = updated.(Model)
					m.CurrentTab = tab
					dialog.setup(&m)

					for _, line := range strings.Split(m.View().Content, "\n") {
						if got := ansi.StringWidth(line); got > width {
							t.Fatalf("line width = %d, want <= %d: %q", got, width, line)
						}
					}
				})
			}
		}
	}
}

func TestOverlayModalsRespectPhysicalViewport(t *testing.T) {
	modals := []struct {
		name  string
		setup func(*Model)
	}{
		{
			name: "dependency update",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogUpdate, ChoiceYes: true}
			},
		},
		{
			name: "dependency checks",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
			},
		},
		{
			name: "dependency rollback",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{
					Kind:      DialogRollback,
					ChoiceYes: true,
					CheckResult: &deps.DependencyCheckResult{
						Command: "go test ./...",
						Output: strings.Join([]string{
							"FAIL: example.com/module/package",
							"expected: successful update",
							"actual: failed dependency check",
						}, "\n"),
					},
				}
			},
		},
		{
			name: "dependency restore",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogRestore, ChoiceYes: true}
				m.Deps.Backups = []deps.DependencyBackupInfo{{
					Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
					Kind:    deps.DependencyBackupKindPreUpdate,
					Updated: 1,
				}}
			},
		},
		{
			name: "dependency backup limit",
			setup: func(m *Model) {
				m.CurrentTab = SettingsTab
				m.Settings.Cursor = 2
				m.Settings.OpenDepsBackupLimitInput()
			},
		},
	}
	viewports := []struct {
		name          string
		width, height int
	}{
		{name: "64x20", width: 64, height: 20},
		{name: "80x30", width: 80, height: 30},
		{name: "wide", width: 140, height: 40},
	}

	for _, viewport := range viewports {
		for _, modal := range modals {
			t.Run(viewport.name+"/"+modal.name, func(t *testing.T) {
				m := newTestModel(t)
				updated, _ := m.Update(tea.WindowSizeMsg{
					Width:  viewport.width,
					Height: viewport.height,
				})
				m = updated.(Model)
				modal.setup(&m)

				content := m.View().Content
				lines := strings.Split(content, "\n")
				if len(lines) > viewport.height {
					t.Fatalf("view height = %d, want <= %d:\n%s", len(lines), viewport.height, content)
				}
				for _, line := range lines {
					if got := ansi.StringWidth(line); got > viewport.width {
						t.Fatalf("line width = %d, want <= %d: %q", got, viewport.width, line)
					}
				}

				if modal.name == "dependency backup limit" {
					if !strings.Contains(content, "enter: save  esc: cancel") {
						t.Fatalf("backup-limit dialog footer is missing:\n%s", content)
					}
					if !strings.Contains(content, "╰") {
						t.Fatalf("backup-limit dialog bottom border is missing:\n%s", content)
					}
				}
			})
		}
	}
}

func TestRollbackDialogLimitsLongOutput(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)
	output := make([]string, maxDependencyListLines+4)
	for i := range output {
		output[i] = "failure output line " + strconv.Itoa(i+1)
	}
	m.Deps.Dialog = ConfirmDialog{
		Kind:      DialogRollback,
		ChoiceYes: true,
		CheckResult: &deps.DependencyCheckResult{
			Command: "go test ./...",
			Output:  strings.Join(output, "\n"),
		},
	}

	content := m.View().Content
	want := "…and 4 more"
	if !strings.Contains(content, want) {
		t.Fatalf("rollback dialog is missing truncated-output indicator %q:\n%s", want, content)
	}
}

func TestViewHasEqualHeightAcrossTabsAtStandardViewport(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	var lineCount int
	for tab := AvailableTab; tab < tabCount; tab++ {
		m.CurrentTab = tab
		got := len(strings.Split(m.View().Content, "\n"))
		if tab == AvailableTab {
			lineCount = got
			continue
		}
		if got != lineCount {
			t.Fatalf("tab %d view line count = %d, want %d", tab, got, lineCount)
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
				m.Deps.Dialog = ConfirmDialog{
					Kind:      DialogUpdate,
					ChoiceYes: true,
					UpdateEntries: []deps.DependencyUpdateEntry{{
						Path:       "github.com/acme/very-long-module-name-that-must-not-overflow-the-terminal",
						OldVersion: "v1.0.0",
						NewVersion: "v1.1.0",
					}},
				}
			},
		},
		{
			name: "checks",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogChecks, ChoiceYes: true}
			},
		},
		{
			name: "rollback",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{
					Kind:      DialogRollback,
					ChoiceYes: true,
					CheckResult: &deps.DependencyCheckResult{
						Command: "go test ./...",
						Output:  strings.Repeat("failure output ", 12),
					},
				}
			},
		},
		{
			name: "restore",
			setup: func(m *Model) {
				m.Deps.Dialog = ConfirmDialog{Kind: DialogRestore, ChoiceYes: true}
				m.Deps.Backups = []deps.DependencyBackupInfo{{
					Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
					Kind:    deps.DependencyBackupKindPreUpdate,
					Updated: 1,
				}}
			},
		},
	}

	for _, width := range []int{64, 80, 120} {
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
