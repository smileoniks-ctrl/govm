package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) View() tea.View {
	appStyle := styles.AppStyleFor(m.Layout)
	width := m.viewWidth()
	height := m.viewHeight()
	viewport := viewportSize{Width: width, Height: height}
	if m.TermWidth > 0 {
		viewport.Width = m.TermWidth
	}
	if m.TermHeight > 0 {
		viewport.Height = m.TermHeight
	}
	if (m.TermWidth > 0 || m.TermHeight > 0 || (m.Width == 1 && m.Height == 1)) &&
		(m.TermWidth < styles.MinTermWidth || m.TermHeight < styles.MinTermHeight) {
		v := tea.NewView(renderMinimumViewport(m.TermWidth, m.TermHeight))
		v.BackgroundColor = styles.MinimumViewportBackground
		v.AltScreen = true
		return v
	}

	components := make([]string, 0, 6)
	components = append(components, renderHeader(width))
	components = append(components, renderTabs(m.CurrentTab))

	if m.ShimPathWarning != "" {
		components = append(components, renderStatus("warning", m.ShimPathWarning, width))
	}

	switch m.CurrentTab {
	case AvailableTab:
		components = append(components, renderContentCanvas(m.List.View(), width, height))
	case InstalledTab:
		components = append(components, renderContentCanvas(m.InstalledTable.View(), width, height))
	case DepsTab:
		components = append(components, renderContentCanvas(m.Deps.Table.View(), width, height))
	case SettingsTab:
		components = append(components, renderContentCanvas(renderSettingsView(m.Settings), width, height))
	}

	if status, statusType := m.composeStatus(); status != "" {
		components = append(components, renderStatus(statusType, status, width))
	}

	help := renderHelp(
		m.CurrentTab,
		m.ConfirmingDelete,
		m.Deps.Dialog,
		width,
	)
	if m.Settings.EditingDepsBackupLimit {
		help = renderKeyHints([][2]string{
			{"enter", "save"},
			{"esc", "cancel"},
		}, width)
	}
	components = append(components, help)
	rendered := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, components...))

	if m.Settings.EditingDepsBackupLimit {
		rendered = overlayDialog(rendered, renderDepsBackupLimitDialog(m.Settings, viewport), viewport)
	} else if m.Deps.Dialog.Active() {
		rendered = overlayDialog(rendered, m.Deps.Dialog.Render(m.Deps, viewport), viewport)
	}

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
}

func renderMinimumViewport(width, height int) string {
	width = maxInt(1, width)
	height = maxInt(1, height)

	lines := []string{
		fmt.Sprintf("Minimum terminal size is %dx%d.", styles.MinTermWidth, styles.MinTermHeight),
		fmt.Sprintf("Current size: %dx%d.", width, height),
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, width)
	}

	message := lipgloss.NewStyle().
		Foreground(styles.MinimumViewportText).
		Render(strings.Join(lines, "\n"))
	background := lipgloss.NewStyle().Background(styles.MinimumViewportBackground)
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		message,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(background),
	)
}

// composeStatus returns the current status message and type, taking
// loading/spinner state into account so the caller doesn't have to.
func (m Model) composeStatus() (string, string) {
	status := m.Message
	statusType := m.MessageType
	if m.Loading || m.Deps.Checking || m.Deps.Updating || m.Deps.RunningChecks || m.Deps.RollingBack || m.Deps.LoadingBackups || m.Deps.RestoringBackup {
		statusType = "info"
		if m.InstallingVersion != "" {
			status = fmt.Sprintf("%s Downloading Go %s", m.Spinner.View(), m.InstallingVersion)
		} else if m.Deps.RollingBack {
			status = fmt.Sprintf("%s Rolling back dependencies", m.Spinner.View())
		} else if m.Deps.RestoringBackup {
			status = fmt.Sprintf("%s Restoring dependency backup", m.Spinner.View())
		} else if m.Deps.RunningChecks {
			status = fmt.Sprintf("%s Running checks", m.Spinner.View())
		} else if m.Deps.LoadingBackups {
			status = fmt.Sprintf("%s Loading dependency backups", m.Spinner.View())
		} else if status == "" {
			status = fmt.Sprintf("%s Loading", m.Spinner.View())
		}
	}
	return status, statusType
}

func renderHeader(width int) string {
	title := styles.TitleStyle.Render("GoVM")

	meta := styles.HeaderMetaStyle.Render(fmt.Sprintf("Go Version Manager %s", utils.GetVersion()))
	spacerWidth := maxInt(1, width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", spacerWidth), meta)
}

func renderTabs(currentTab int) string {
	tabs := []string{
		renderTab("Available", currentTab == AvailableTab),
		renderTab("Installed", currentTab == InstalledTab),
		renderTab("Deps", currentTab == DepsTab),
		renderTab("Settings", currentTab == SettingsTab),
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

func renderSettingsView(settings SettingsState) string {
	values := config.Normalize(settings.Values)
	rows := []string{
		fmt.Sprintf("Deps display: %s", depsDisplayLabel(values.DepsDisplay)),
		fmt.Sprintf("Theme: %s", themeLabel(values.Theme)),
		fmt.Sprintf("Deps backups: %d", values.DepsBackupLimit),
	}
	for i, row := range rows {
		prefix := "  "
		if i == settings.Cursor {
			prefix = "> "
		}
		rows[i] = prefix + row
	}
	return strings.Join(rows, "\n")
}

func depsDisplayLabel(mode config.DepsDisplayMode) string {
	if mode == config.DepsDisplayAll {
		return "All"
	}
	return "Direct only"
}

func themeLabel(name config.ThemeName) string {
	if name == config.ThemeLight {
		return "Light"
	}
	return "Current"
}

func renderHelp(currentTab int, confirmingDelete bool, dialog ConfirmDialog, width int) string {
	var hints [][2]string

	switch dialog.Kind {
	case DialogUpdate:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "cancel"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width)
	case DialogChecks:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "skip"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width)
	case DialogRollback:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width)
	case DialogRestore:
		action := "cancel"
		if dialog.ChoiceYes {
			action = "restore"
		}
		hints = [][2]string{
			{"↑/↓", "select"},
			{"←/→", "choose"},
			{"enter", action},
			{"esc", "cancel"},
		}
		return renderKeyHints(hints, width)
	}

	if confirmingDelete {
		hints = [][2]string{
			{"y", "confirm"},
			{"n", "cancel"},
			{"q", "quit"},
		}
	} else if currentTab == AvailableTab {
		hints = [][2]string{
			{"i", "install"},
			{"u", "use"},
			{"d", "delete"},
			{"r", "refresh"},
			{"tab", "switch"},
			{"q", "quit"},
		}
	} else if currentTab == DepsTab {
		hints = [][2]string{
			{"r", "check updates"},
			{"u", "update"},
			{"b", "backups"},
			{"tab", "switch"},
			{"q", "quit"},
		}
	} else if currentTab == SettingsTab {
		hints = [][2]string{
			{"↑/↓", "move"},
			{"enter", "toggle"},
			{"tab", "switch"},
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

	return renderKeyHints(hints, width)
}

func renderKeyHints(hints [][2]string, width int) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, fmt.Sprintf("%s %s", styles.HelpKeyStyle.Render(hint[0]), styles.HelpTextStyle.Render(hint[1])))
	}

	helpText := strings.Join(parts, "  ")

	if lipgloss.Width(helpText) > width {
		helpText = styles.TruncateText(helpText, width)
	}

	return helpText
}

func renderContentCanvas(content string, width, height int) string {
	if height < 1 {
		return ""
	}

	content = strings.TrimRight(content, "\n")
	var canvas strings.Builder
	canvas.Grow(height * (maxInt(0, width) + 1))

	lineStart := 0
	for row := 0; row < height; row++ {
		var line string
		if lineStart < len(content) {
			lineEnd := strings.IndexByte(content[lineStart:], '\n')
			if lineEnd < 0 {
				line = content[lineStart:]
				lineStart = len(content)
			} else {
				lineEnd += lineStart
				line = content[lineStart:lineEnd]
				lineStart = lineEnd + 1
			}
		}
		canvas.WriteString(line)
		if padding := width - ansi.StringWidth(line); padding > 0 {
			for range padding {
				canvas.WriteByte(' ')
			}
		}
		if row+1 < height {
			canvas.WriteByte('\n')
		}
	}
	return canvas.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
