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

var (
	Primary = lipgloss.Color("#7C3AED")
	Success = lipgloss.Color("#10B981")
	Error   = lipgloss.Color("#EF4444")
	Warning = lipgloss.Color("#F59E0B")
	Info    = lipgloss.Color("#3B82F6")
	Muted   = lipgloss.Color("#6B7280")
	Text    = lipgloss.Color("#E5E7EB")

	MinimumViewportBackground = lipgloss.Color("#111827")
	MinimumViewportText       = lipgloss.Color("#F9FAFB")

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Text)

	HeaderMetaStyle = lipgloss.NewStyle().Foreground(Muted)

	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(Primary).
			Bold(true).
			Padding(0, 1)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(Muted).
				Padding(0, 1)

	ActiveBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#052E16")).
				Background(Success).
				Bold(true).
				Padding(0, 1)

	InstalledBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F8FAFC")).
				Background(Primary).
				Padding(0, 1)

	ItemVersionStyle = lipgloss.NewStyle().Foreground(Text).Bold(true)
	MutedStyle       = lipgloss.NewStyle().Foreground(Muted)

	StatusSuccessStyle = lipgloss.NewStyle().Foreground(Success).Bold(true)
	StatusErrorStyle   = lipgloss.NewStyle().Foreground(Error).Bold(true)
	StatusWarningStyle = lipgloss.NewStyle().Foreground(Warning).Bold(true)
	StatusInfoStyle    = lipgloss.NewStyle().Foreground(Info).Bold(true)

	HelpKeyStyle  = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	HelpTextStyle = lipgloss.NewStyle().Foreground(Muted)

	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(Primary).
				Padding(0, 1)

	TableSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(Primary).
				Bold(true).
				Padding(0, 1)

	TableCellStyle = lipgloss.NewStyle().Padding(0, 1)
	SpinnerStyle   = lipgloss.NewStyle().Foreground(Primary)
)

func AppStyleFor(mode LayoutMode) lipgloss.Style {
	switch mode {
	case LayoutWide:
		return lipgloss.NewStyle().
			Padding(1, 2).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(Primary)
	default:
		return lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(Primary)
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
