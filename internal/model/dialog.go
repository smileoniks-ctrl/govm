package model

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

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

// renderDialog wraps content in the themed dialog border box. It takes
// the theme as a parameter rather than reading package-level style
// state; the previous init()/rebuildDialogStyles machinery is gone
// along with the style vars it maintained.
func renderDialog(t styles.Theme, content string, errorStyle bool, viewport viewportSize) string {
	width := dialogWidth(viewport)
	contentWidth := max(1, width-6)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, contentWidth)
	}

	style := t.DialogBoxStyle
	if errorStyle {
		style = t.DialogErrorBoxStyle
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
