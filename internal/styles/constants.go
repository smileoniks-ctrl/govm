package styles

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	WideBreakpoint = 130
	MinTermWidth   = 64
	MinTermHeight  = 20
)

type LayoutMode int

const (
	LayoutNormal LayoutMode = iota
	LayoutWide
)

func GetLayoutMode(width int) LayoutMode {
	if width >= WideBreakpoint {
		return LayoutWide
	}
	return LayoutNormal
}

// AppStyleFor returns the outer frame style for the given layout mode,
// themed by t.Primary. It lives on Theme so renderers receive a single
// value instead of reading package-level state.
func (t Theme) AppStyleFor(mode LayoutMode) lipgloss.Style {
	border := lipgloss.NormalBorder()
	switch mode {
	case LayoutWide:
		return lipgloss.NewStyle().
			Padding(1, 2).
			BorderStyle(border).
			BorderForeground(t.Primary)
	default:
		return lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(border).
			BorderForeground(t.Primary)
	}
}

func FrameOverhead(mode LayoutMode) (horizontal, vertical int) {
	switch mode {
	case LayoutWide:
		return 6, 4
	default:
		return 4, 2
	}
}

func TruncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= maxWidth {
		return text
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}
	return lipgloss.NewStyle().MaxWidth(maxWidth).Render(text)
}
