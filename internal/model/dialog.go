package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// Static styles for the dependency update dialogs. These are computed
// once at package init rather than on every render call, which
// previously recreated four `lipgloss.NewStyle()` values plus the
// wrapper border per View().
var (
	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(styles.Warning).
				Padding(0, 1)

	dialogWarningStyle = styles.StatusWarningStyle
	dialogBodyStyle    = lipgloss.NewStyle().Padding(0, 1)
	dialogMutedStyle   = lipgloss.NewStyle().
				Foreground(styles.Muted).
				Padding(0, 1)

	dialogActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(styles.Primary).
				Bold(true).
				Padding(0, 2)

	dialogInactiveStyle = lipgloss.NewStyle().
				Foreground(styles.Muted).
				Padding(0, 2)

	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Warning).
			Padding(1, 2).
			Width(64)

	dialogErrorBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.Error).
				Padding(1, 2).
				Width(64)
)

const maxDependencyListLines = 6

func renderDependencyUpdateDialog(yesSelected bool, updatable []utils.DependencyUpdateEntry) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Да"),
		"  ",
		noBtn.Render("Нет"),
	)

	lines := make([]string, 0, 6+len(updatable))
	lines = append(lines, dialogTitleStyle.Render(dialogWarningStyle.Render("⚠ Warning")))
	lines = append(lines, "")
	lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf(
		"Будут обновлены %d %s:",
		len(updatable),
		pluralize(len(updatable), "прямая зависимость", "прямые зависимости"),
	)))

	visible := updatable
	extra := 0
	if len(visible) > maxDependencyListLines {
		extra = len(visible) - maxDependencyListLines
		visible = visible[:maxDependencyListLines]
	}
	for _, e := range visible {
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf(
			"  %s: %s -> %s", e.Path, e.OldVersion, e.NewVersion,
		)))
	}
	if extra > 0 {
		lines = append(lines, dialogBodyStyle.Render(
			fmt.Sprintf("  …и ещё %d", extra),
		))
	}
	lines = append(lines, "")
	lines = append(lines, dialogBodyStyle.Render("Файлы go.mod и go.sum будут изменены."))
	lines = append(lines, dialogBodyStyle.Render("Перед обновлением сохраняется снимок, чтобы можно было откатить изменения."))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return dialogBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func renderDependencyChecksDialog(yesSelected bool) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Да"),
		"  ",
		noBtn.Render("Нет"),
	)

	lines := []string{
		dialogTitleStyle.Render(styles.StatusInfoStyle.Render("✓ Запустить проверки?")),
		"",
		dialogBodyStyle.Render("После обновления будут выполнены:"),
		dialogBodyStyle.Render("  • go test ./..."),
		dialogBodyStyle.Render("  • go vet ./..."),
		"",
		dialogMutedStyle.Render("При провале будет предложено откатить зависимости."),
		"",
		buttons,
	}

	return dialogBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func renderDependencyRollbackDialog(yesSelected bool, result *utils.DependencyCheckResultMsg) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Откатить"),
		"  ",
		noBtn.Render("Оставить"),
	)

	lines := []string{
		dialogTitleStyle.Render(dialogWarningStyle.Render("⚠ Проверки провалились")),
		"",
	}
	if result != nil {
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf("Команда: %s", result.Command)))
		if result.Output != "" {
			for _, l := range strings.Split(result.Output, "\n") {
				lines = append(lines, dialogMutedStyle.Render(l))
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, dialogBodyStyle.Render("Откатить зависимости к состоянию до обновления?"))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return dialogErrorBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func overlayDialog(background, dialog string, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
	)

	bgLines := strings.Split(background, "\n")
	dialogLines := strings.Split(placed, "\n")

	// Center the dialog lines over the background lines.
	startRow := 0
	if len(bgLines) > len(dialogLines) {
		startRow = (len(bgLines) - len(dialogLines)) / 2
	}
	endRow := startRow + len(dialogLines)
	if endRow > len(bgLines) {
		endRow = len(bgLines)
	}
	dialogLines = dialogLines[:endRow-startRow]

	for i, dline := range dialogLines {
		row := startRow + i
		bgLine := bgLines[row]
		bgW := lipgloss.Width(bgLine)
		dW := lipgloss.Width(dline)
		col := 0
		if bgW > dW {
			col = (bgW - dW) / 2
		}
		bgLines[row] = spliceCentered(bgLine, dline, col)
	}

	return strings.Join(bgLines, "\n")
}

func spliceCentered(bg, overlay string, col int) string {
	if col < 0 {
		col = 0
	}
	bgRunes := []rune(bg)
	ovRunes := []rune(overlay)
	if col > len(bgRunes) {
		col = len(bgRunes)
	}

	prefix := string(bgRunes[:col])
	suffix := ""
	if col+len(ovRunes) < len(bgRunes) {
		suffix = string(bgRunes[col+len(ovRunes):])
	}
	return prefix + overlay + suffix
}
