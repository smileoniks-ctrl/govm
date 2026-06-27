package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// Static styles for the dependency update dialog. These are computed
// once at package init rather than on every renderDependencyUpdateDialog
// call, which previously recreated four `lipgloss.NewStyle()` values
// plus the wrapper border per View().
var (
	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(styles.Warning).
				Padding(0, 1)

	dialogWarningStyle = styles.StatusWarningStyle
	dialogBodyStyle    = lipgloss.NewStyle().Padding(0, 1)

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
			Width(54)
)

func renderDependencyUpdateDialog(yesSelected bool) string {
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

	dialog := lipgloss.JoinVertical(lipgloss.Center,
		dialogTitleStyle.Render(dialogWarningStyle.Render("⚠ Warning")),
		"",
		dialogBodyStyle.Render("Вы уверены что хотите обновить зависимости?"),
		"",
		buttons,
	)

	return dialogBoxStyle.Render(dialog)
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
