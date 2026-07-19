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

// defaultConstructionWidth is the initial column budget passed to the
// table-column helpers when a real WindowSizeMsg has not arrived yet.
// It mirrors the fallback returned by Model.viewWidth.
const defaultConstructionWidth = 80

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

// New builds the top-level Model for the TUI. It owns the invariant
// setup that every caller needs: the spinner, the installed-versions
// and dependencies tables (columns + height + styles), the version
// list and its delegate, and the Deps/Settings sub-states.
//
// Callers (main, tests, benchmarks) receive a Model with the default
// start state and may override any field afterwards to vary the
// scenario. Fields intentionally initialised here represent
// invariants of the TUI layout, not user-tunable knobs.
func New(moduleDir, settingsPath string, settings config.Settings, shimPathWarning string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.SpinnerStyle

	installedTable := table.New(
		table.WithColumns(installedTableColumns(defaultConstructionWidth)),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	installedTable.SetStyles(table.Styles{
		Header:   styles.TableHeaderStyle,
		Selected: styles.TableSelectedStyle,
		Cell:     styles.TableCellStyle,
	})

	depTable := table.New(
		table.WithColumns(dependencyTableColumns(defaultConstructionWidth)),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	depTable.SetStyles(table.Styles{
		Header:   styles.TableHeaderStyle,
		Selected: styles.TableSelectedStyle,
		Cell:     styles.TableCellStyle,
	})

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = styles.TableSelectedStyle
	delegate.Styles.SelectedDesc = styles.TableSelectedStyle
	delegate.Styles.NormalDesc = styles.MutedStyle

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Available Versions"
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)

	return Model{
		List:            l,
		Versions:        []utils.GoVersion{},
		Spinner:         sp,
		Loading:         true,
		InstalledTable:  installedTable,
		Layout:          styles.LayoutNormal,
		Deps:            NewDepsState(moduleDir, depTable),
		Settings:        NewSettingsState(settingsPath, settings),
		ShimPathWarning: shimPathWarning,
	}
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
