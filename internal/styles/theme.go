package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type ThemeName string

const (
	ThemeCurrent ThemeName = "current"
	ThemeLight   ThemeName = "light"
)

type themePalette struct {
	Primary                  color.Color
	Success                  color.Color
	Error                    color.Color
	Warning                  color.Color
	Info                     color.Color
	Muted                    color.Color
	Text                     color.Color
	OnPrimary                color.Color
	InstalledBadgeForeground color.Color
}

var (
	currentTheme = ThemeCurrent

	themeRegistry = map[ThemeName]themePalette{
		ThemeCurrent: {
			Primary:                  lipgloss.Color("#7C3AED"),
			Success:                  lipgloss.Color("#10B981"),
			Error:                    lipgloss.Color("#EF4444"),
			Warning:                  lipgloss.Color("#F59E0B"),
			Info:                     lipgloss.Color("#3B82F6"),
			Muted:                    lipgloss.Color("#6B7280"),
			Text:                     lipgloss.Color("#E5E7EB"),
			OnPrimary:                lipgloss.Color("#FFFFFF"),
			InstalledBadgeForeground: lipgloss.Color("#F8FAFC"),
		},
		ThemeLight: {
			Primary:                  lipgloss.Color("#6D28D9"),
			Success:                  lipgloss.Color("#047857"),
			Error:                    lipgloss.Color("#DC2626"),
			Warning:                  lipgloss.Color("#B45309"),
			Info:                     lipgloss.Color("#2563EB"),
			Muted:                    lipgloss.Color("#4B5563"),
			Text:                     lipgloss.Color("#111827"),
			OnPrimary:                lipgloss.Color("#FFFFFF"),
			InstalledBadgeForeground: lipgloss.Color("#F8FAFC"),
		},
	}
)

func ApplyTheme(name ThemeName) ThemeName {
	palette, ok := themeRegistry[name]
	if !ok {
		name = ThemeCurrent
		palette = themeRegistry[ThemeCurrent]
	}

	currentTheme = name
	rebuildStyles(palette)
	return currentTheme
}

func CurrentTheme() ThemeName {
	return currentTheme
}

func rebuildStyles(palette themePalette) {
	Primary = palette.Primary
	Success = palette.Success
	Error = palette.Error
	Warning = palette.Warning
	Info = palette.Info
	Muted = palette.Muted
	Text = palette.Text

	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(Text)

	HeaderMetaStyle = lipgloss.NewStyle().Foreground(Muted)

	ActiveTabStyle = lipgloss.NewStyle().
		Foreground(palette.OnPrimary).
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
		Foreground(palette.InstalledBadgeForeground).
		Background(Primary).
		Padding(0, 1)

	ItemVersionStyle = lipgloss.NewStyle().Foreground(Text).Bold(true)
	MutedStyle = lipgloss.NewStyle().Foreground(Muted)

	StatusSuccessStyle = lipgloss.NewStyle().Foreground(Success).Bold(true)
	StatusErrorStyle = lipgloss.NewStyle().Foreground(Error).Bold(true)
	StatusWarningStyle = lipgloss.NewStyle().Foreground(Warning).Bold(true)
	StatusInfoStyle = lipgloss.NewStyle().Foreground(Info).Bold(true)

	HelpKeyStyle = lipgloss.NewStyle().Foreground(Primary).Bold(true)
	HelpTextStyle = lipgloss.NewStyle().Foreground(Muted)

	TableHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.OnPrimary).
		Background(Primary).
		Padding(0, 1)

	TableSelectedStyle = lipgloss.NewStyle().
		Foreground(palette.OnPrimary).
		Background(Primary).
		Bold(true).
		Padding(0, 1)

	TableCellStyle = lipgloss.NewStyle().Padding(0, 1)
	SpinnerStyle = lipgloss.NewStyle().Foreground(Primary)
}
