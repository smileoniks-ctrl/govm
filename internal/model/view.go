package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) View() tea.View {
	appStyle := styles.AppStyleFor(m.Layout)
	width := m.viewWidth()
	height := m.viewHeight()

	if m.Err != nil {
		return tea.NewView(appStyle.Render(renderStatus("error", fmt.Sprintf("Error: %s", m.Err), width)))
	}

	components := make([]string, 0, 6)
	components = append(components, renderHeader(width, m.Layout))
	components = append(components, renderTabs(m.CurrentTab, m.Layout))

	if !utils.IsShimInPath() {
		components = append(components, renderStatus("warning", "GoVM is not in your PATH. "+utils.GetShimPathInstructions(), width))
	}

	switch m.CurrentTab {
	case 0:
		components = append(components, m.List.View())
	case 1:
		components = append(components, m.InstalledTable.View())
	case 2:
		components = append(components, m.DependencyTable.View())
	}

	if status, statusType := m.composeStatus(); status != "" {
		components = append(components, renderStatus(statusType, status, width))
	}

	components = append(components, renderHelp(m.CurrentTab, m.ConfirmingDelete, m.ConfirmingDependencyUpdate, m.ConfirmingDependencyChecks, m.ConfirmingDependencyRollback, width, m.Layout))
	rendered := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, components...))

	if m.ConfirmingDependencyUpdate {
		updatable := utils.UpdatableDirectDependencies(m.Dependencies)
		entries := make([]utils.DependencyUpdateEntry, 0, len(updatable))
		for _, d := range updatable {
			entries = append(entries, utils.DependencyUpdateEntry{
				Path:       d.Path,
				OldVersion: d.Version,
				NewVersion: d.Latest,
			})
		}
		rendered = overlayDialog(rendered, renderDependencyUpdateDialog(m.UpdateChoiceYes, entries), width, height)
	} else if m.ConfirmingDependencyChecks {
		rendered = overlayDialog(rendered, renderDependencyChecksDialog(m.CheckChoiceYes), width, height)
	} else if m.ConfirmingDependencyRollback {
		rendered = overlayDialog(rendered, renderDependencyRollbackDialog(m.RollbackChoiceYes, m.LastCheckResult), width, height)
	}

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
}

// composeStatus returns the current status message and type, taking
// loading/spinner state into account so the caller doesn't have to.
func (m Model) composeStatus() (string, string) {
	status := m.Message
	statusType := m.MessageType
	if m.Loading || m.CheckingDependencies || m.UpdatingDependencies || m.RunningDependencyChecks || m.RollingBackDependencies {
		statusType = "info"
		if m.InstallingVersion != "" {
			status = fmt.Sprintf("%s Downloading Go %s", m.Spinner.View(), m.InstallingVersion)
		} else if m.RollingBackDependencies {
			status = fmt.Sprintf("%s Rolling back dependencies", m.Spinner.View())
		} else if m.RunningDependencyChecks {
			status = fmt.Sprintf("%s Running checks", m.Spinner.View())
		} else if status == "" {
			status = fmt.Sprintf("%s Loading", m.Spinner.View())
		}
	}
	return status, statusType
}

func renderHeader(width int, layout styles.LayoutMode) string {
	title := styles.TitleStyle.Render("GoVM")

	if layout == styles.LayoutCompact {
		return title
	}

	meta := styles.HeaderMetaStyle.Render(fmt.Sprintf("Go Version Manager %s", utils.GetVersion()))
	spacerWidth := maxInt(1, width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", spacerWidth), meta)
}

func renderTabs(currentTab int, layout styles.LayoutMode) string {
	var availableLabel, installedLabel, depsLabel string
	if layout == styles.LayoutCompact {
		availableLabel, installedLabel, depsLabel = "All", "Local", "Deps"
	} else {
		availableLabel, installedLabel, depsLabel = "Available", "Installed", "Deps"
	}

	tabs := []string{
		renderTab(availableLabel, currentTab == 0),
		renderTab(installedLabel, currentTab == 1),
		renderTab(depsLabel, currentTab == 2),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
}

func renderTab(label string, active bool) string {
	if active {
		return styles.ActiveTabStyle.Render("● " + label)
	}
	return styles.InactiveTabStyle.Render("○ " + label)
}

func renderStatus(messageType, message string, width int) string {
	if message == "" {
		return ""
	}

	icon := "•"
	style := styles.StatusInfoStyle
	switch messageType {
	case "success":
		icon = "✓"
		style = styles.StatusSuccessStyle
	case "error":
		icon = "✕"
		style = styles.StatusErrorStyle
	case "warning":
		icon = "!"
		style = styles.StatusWarningStyle
	case "info":
		icon = "•"
		style = styles.StatusInfoStyle
	}

	return style.Width(width).Render(fmt.Sprintf("%s %s", icon, message))
}

func renderHelp(currentTab int, confirmingDelete, confirmingDeps, confirmingChecks, confirmingRollback bool, width int, layout styles.LayoutMode) string {
	var hints [][2]string

	switch {
	case confirmingDeps:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "cancel"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width, layout)
	case confirmingChecks:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "skip"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width, layout)
	case confirmingRollback:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width, layout)
	}

	if confirmingDelete {
		hints = [][2]string{
			{"y", "confirm"},
			{"n", "cancel"},
			{"q", "quit"},
		}
	} else if currentTab == 0 {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"i", "inst"},
				{"u", "use"},
				{"d", "del"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"i", "install"},
				{"u", "use"},
				{"d", "delete"},
				{"r", "refresh"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	} else if currentTab == 2 {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"r", "check"},
				{"u", "update"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"r", "check updates"},
				{"u", "update"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	} else {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"u", "use"},
				{"d", "del"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"u", "use"},
				{"d", "delete"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	}

	return renderKeyHints(hints, width, layout)
}

func renderKeyHints(hints [][2]string, width int, layout styles.LayoutMode) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, fmt.Sprintf("%s %s", styles.HelpKeyStyle.Render(hint[0]), styles.HelpTextStyle.Render(hint[1])))
	}

	helpText := strings.Join(parts, "  ")

	if layout == styles.LayoutCompact && lipgloss.Width(helpText) > width {
		helpText = strings.Join(parts, " ")
	}

	if lipgloss.Width(helpText) > width {
		helpText = styles.TruncateText(helpText, width)
	}

	return helpText
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
