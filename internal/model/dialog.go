package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// Static styles for modal dialogs. They are recomputed only when the user
// changes the active theme.
var (
	dialogTitleStyle    lipgloss.Style
	dialogWarningStyle  lipgloss.Style
	dialogBodyStyle     lipgloss.Style
	dialogMutedStyle    lipgloss.Style
	dialogActiveStyle   lipgloss.Style
	dialogInactiveStyle lipgloss.Style
	dialogBoxStyle      lipgloss.Style
	dialogErrorBoxStyle lipgloss.Style
)

func init() {
	rebuildDialogStyles()
}

func rebuildDialogStyles() {
	dialogTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.Warning).
		Padding(0, 1)

	dialogWarningStyle = styles.StatusWarningStyle
	dialogBodyStyle = lipgloss.NewStyle().Padding(0, 1)
	dialogMutedStyle = lipgloss.NewStyle().
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
}

const maxDependencyListLines = 6

type viewportSize struct {
	Width  int
	Height int
}

func dialogWidth(viewport viewportSize) int {
	width := viewport.Width
	if width < 1 {
		width = 64
	}
	return min(64, width)
}

func renderDialog(content string, errorStyle bool, viewport viewportSize) string {
	width := dialogWidth(viewport)
	contentWidth := max(1, width-6)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, contentWidth)
	}

	style := dialogBoxStyle
	if errorStyle {
		style = dialogErrorBoxStyle
	}
	return style.Width(width).Render(strings.Join(lines, "\n"))
}

func overlayDialog(background, dialog string, viewport viewportSize) string {
	width, height := viewport.Width, viewport.Height
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	dialogLines := strings.Split(strings.TrimRight(dialog, "\n"), "\n")
	bgLines := strings.Split(background, "\n")
	if len(bgLines) > height {
		bgLines = bgLines[:height]
	}
	blankRow := strings.Repeat(" ", width)
	for len(bgLines) < height {
		bgLines = append(bgLines, blankRow)
	}

	startRow := 0
	if height > len(dialogLines) {
		startRow = (height - len(dialogLines)) / 2
	}
	if startRow+len(dialogLines) > height {
		startRow = height - len(dialogLines)
		if startRow < 0 {
			startRow = 0
		}
	}
	endRow := startRow + len(dialogLines)
	if endRow > height {
		endRow = height
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
		bgLines[row] = spliceCentered(bgLine, dline, col, bgW, dW)
	}

	return strings.Join(bgLines, "\n")
}

func spliceCentered(bg, overlay string, col, bgW, overlayW int) string {
	if col < 0 {
		col = 0
	}
	// col is a column index measured in visible cells (the value the caller
	// got from ansi.StringWidth / lipgloss.Width). Slicing by []rune would
	// (a) drop a wide rune that straddles the cut point and (b) chop ANSI
	// escape sequences in half, which corrupts the surrounding styled
	// table output. Use ANSI-aware cuts instead.
	if col > bgW {
		col = bgW
	}

	prefix := ansi.Cut(bg, 0, col)
	suffix := ""
	if col+overlayW < bgW {
		suffix = ansi.Cut(bg, col+overlayW, bgW)
	}
	return prefix + overlay + suffix
}
