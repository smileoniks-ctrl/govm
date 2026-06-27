package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	_ = width

	// The dialog already has its own border/padding rendered by
	// dialogBoxStyle; we must not pad it out to a full-height canvas,
	// otherwise every background line outside the modal gets overwritten
	// with whitespace (the bug behind CleanShot 2026-06-27 at 17.54.47@2x.png
	// where the deps table was shredded by the confirm dialog).
	dialogLines := strings.Split(strings.TrimRight(dialog, "\n"), "\n")
	bgLines := strings.Split(background, "\n")
	if len(bgLines) == 0 {
		return background
	}

	if height > len(bgLines) {
		height = len(bgLines)
	}

	// Center the actual dialog lines vertically on the background.
	startRow := 0
	if len(bgLines) > len(dialogLines) {
		startRow = (len(bgLines) - len(dialogLines)) / 2
	}
	if startRow+len(dialogLines) > height {
		// Keep the dialog inside the visible viewport.
		startRow = height - len(dialogLines)
		if startRow < 0 {
			startRow = 0
		}
	}
	endRow := startRow + len(dialogLines)
	if endRow > len(bgLines) {
		endRow = len(bgLines)
	}
	dialogLines = dialogLines[:endRow-startRow]

	for i, dline := range dialogLines {
		row := startRow + i
		bgLine := bgLines[row]
		bgW := ansi.StringWidth(bgLine)
		dW := ansi.StringWidth(dline)
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
	// col is a column index measured in visible cells (the value the caller
	// got from ansi.StringWidth / lipgloss.Width). Slicing by []rune would
	// (a) drop a wide rune that straddles the cut point and (b) chop ANSI
	// escape sequences in half, which corrupts the surrounding styled
	// table output. Use ANSI-aware cuts instead.
	bgW := ansi.StringWidth(bg)
	if col > bgW {
		col = bgW
	}
	ovW := ansi.StringWidth(overlay)

	prefix := ansi.Cut(bg, 0, col)
	suffix := ""
	if col+ovW < bgW {
		suffix = ansi.Cut(bg, col+ovW, bgW)
	}
	return prefix + overlay + suffix
}
