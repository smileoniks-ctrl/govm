package model

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestShouldClampViewWidth(t *testing.T) {
	tests := []struct {
		name                  string
		physicalViewportWidth int
		layout                styles.LayoutMode
		want                  bool
	}{
		{"no physical viewport", 0, styles.LayoutCompact, false},
		{"compact viewport", 30, styles.LayoutCompact, true},
		{"normal viewport", 60, styles.LayoutNormal, false},
		{"wide viewport", 120, styles.LayoutWide, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldClampViewWidth(tt.physicalViewportWidth, tt.layout); got != tt.want {
				t.Fatalf("shouldClampViewWidth(%d, %v) = %t, want %t", tt.physicalViewportWidth, tt.layout, got, tt.want)
			}
		})
	}
}

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
				m.Deps.Dialog.ConfirmingUpdate = true
			},
		},
		{
			name: "checks dialog",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingChecks = true
			},
		},
		{
			name: "rollback dialog",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingRollback = true
				m.Deps.LastCheckResult = &utils.DependencyCheckResultMsg{
					Command: "go test ./...",
					Output:  strings.Repeat("failure output ", 12),
				}
			},
		},
		{
			name: "restore dialog",
			setup: func(m *Model) {
				m.Deps.Dialog.ConfirmingRestoreBackup = true
				m.Deps.Backups = []utils.DependencyBackupInfo{{
					Name:    "2026-07-09_12-00-00-a-very-long-backup-filename.json",
					Kind:    utils.DependencyBackupKindPreUpdate,
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

	for _, width := range []int{30, 59, 60, 120} {
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
