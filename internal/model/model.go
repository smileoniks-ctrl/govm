package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

const (
	AvailableTab = iota
	InstalledTab
	DepsTab
	SettingsTab
	tabCount
)

type Model struct {
	List              list.Model
	Versions          []utils.GoVersion
	Loading           bool
	Spinner           spinner.Model
	CurrentTab        int
	InstallingVersion string
	Message           string
	MessageType       string // "success", "error", "warning", or "info"
	MessageScope      statusScope
	// ShimPathWarning is the pre-rendered PATH warning captured before
	// launching the TUI, so View does not resolve PATH on every render.
	ShimPathWarning  string
	InstalledTable   table.Model
	ConfirmingDelete bool
	DeleteVersion    string
	Width            int
	Height           int
	TermWidth        int
	TermHeight       int
	Layout           styles.LayoutMode

	// Deps groups every field and state machine flag related to the
	// "Deps" tab. Use the helpers in deps_state.go to keep the
	// main Model surface small.
	Deps     DepsState
	Settings SettingsState
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		utils.FetchGoVersions,
		m.Spinner.Tick,
	)
}

func (m Model) viewHeight() int {
	if m.Height > 0 {
		return m.Height
	}
	if m.List.Height() > 0 {
		return m.List.Height()
	}
	return 24
}

func (m Model) viewWidth() int {
	if m.Width > 0 {
		return m.Width
	}
	if m.List.Width() > 0 {
		return m.List.Width()
	}
	return 80
}

func (m Model) normalizedSettings() config.Settings {
	return config.Normalize(m.Settings.Values)
}
