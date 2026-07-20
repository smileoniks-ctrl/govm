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
	t := m.theme
	appStyle := t.AppStyleFor(m.Layout)
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
		v := tea.NewView(renderMinimumViewport(t, m.TermWidth, m.TermHeight))
		v.BackgroundColor = t.MinimumViewportBackground
		v.AltScreen = true
		return v
	}

	components := make([]string, 0, 6)
	components = append(components, renderHeader(t, width))
	components = append(components, renderTabs(t, m.CurrentTab))

	if m.ShimPathWarning != "" {
		components = append(components, renderStatus(t, "warning", m.ShimPathWarning, width))
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
		components = append(components, renderStatus(t, statusType, status, width))
	}

	help := renderHelp(
		t,
		m.CurrentTab,
		m.ConfirmingDelete,
		m.Deps.Dialog,
		width,
	)
	if m.Settings.EditingDepsBackupLimit {
		help = renderKeyHints(t, [][2]string{
			{"enter", "save"},
			{"esc", "cancel"},
		}, width)
	}
	components = append(components, help)
	rendered := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, components...))

	if m.Settings.EditingDepsBackupLimit {
		rendered = overlayDialog(rendered, renderDepsBackupLimitDialog(t, m.Settings, viewport), viewport)
	} else if m.Deps.Dialog.Active() {
		rendered = overlayDialog(rendered, m.Deps.Dialog.Render(t, m.Deps, viewport), viewport)
	}

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
}

func renderMinimumViewport(t styles.Theme, width, height int) string {
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
		Foreground(t.MinimumViewportText).
		Render(strings.Join(lines, "\n"))
	background := lipgloss.NewStyle().Background(t.MinimumViewportBackground)
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
	if m.Loading || m.Deps.operationInProgress() {
		statusType = "info"
		if m.InstallingVersion != "" {
			status = fmt.Sprintf("%s Downloading Go %s", m.Spinner.View(), m.InstallingVersion)
		} else if text := m.Deps.SpinnerText(); text != "" {
			status = fmt.Sprintf("%s %s", m.Spinner.View(), text)
		} else if status == "" {
			status = fmt.Sprintf("%s Loading", m.Spinner.View())
		}
	}
	return status, statusType
}

func renderHeader(t styles.Theme, width int) string {
	title := t.TitleStyle.Render("GoVM")

	meta := t.HeaderMetaStyle.Render(fmt.Sprintf("Go Version Manager %s", utils.GetVersion()))
	spacerWidth := maxInt(1, width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", spacerWidth), meta)
}

func renderTabs(t styles.Theme, currentTab int) string {
	tabs := []string{
		renderTab(t, "Available", currentTab == AvailableTab),
		renderTab(t, "Installed", currentTab == InstalledTab),
		renderTab(t, "Deps", currentTab == DepsTab),
		renderTab(t, "Settings", currentTab == SettingsTab),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
}

func renderTab(t styles.Theme, label string, active bool) string {
	if active {
		return t.ActiveTabStyle.Render("● " + label)
	}
	return t.InactiveTabStyle.Render("○ " + label)
}

func renderStatus(t styles.Theme, messageType, message string, width int) string {
	if message == "" {
		return ""
	}

	icon := "•"
	style := t.StatusInfoStyle
	switch messageType {
	case "success":
		icon = "✓"
		style = t.StatusSuccessStyle
	case "error":
		icon = "✕"
		style = t.StatusErrorStyle
	case "warning":
		icon = "!"
		style = t.StatusWarningStyle
	case "info":
		icon = "•"
		style = t.StatusInfoStyle
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

func renderHelp(t styles.Theme, currentTab int, confirmingDelete bool, dialog ConfirmDialog, width int) string {
	var hints [][2]string

	switch dialog.Kind {
	case DialogUpdate:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "cancel"},
			{"q", "quit"},
		}
		return renderKeyHints(t, hints, width)
	case DialogChecks:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "skip"},
			{"q", "quit"},
		}
		return renderKeyHints(t, hints, width)
	case DialogRollback:
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"q", "quit"},
		}
		return renderKeyHints(t, hints, width)
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
		return renderKeyHints(t, hints, width)
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

	return renderKeyHints(t, hints, width)
}

func renderKeyHints(t styles.Theme, hints [][2]string, width int) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, fmt.Sprintf("%s %s", t.HelpKeyStyle.Render(hint[0]), t.HelpTextStyle.Render(hint[1])))
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
