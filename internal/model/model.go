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
	// catalog is the private Version Catalog that owns the canonical
	// Go version list and produces defensive projections for both
	// version widgets. It replaces the previous public Versions slice
	// and rebuildVersionViews derivation.
	catalog versionCatalog
	// projectionGeneration is bumped on every successful projection
	// sync so deferred refilter restores can detect staleness.
	projectionGeneration uint64
	// pendingListRestore captures the Available-list selection that
	// must be restored once the asynchronous refilter triggered by
	// list.SetItems (while a filter is active) settles. It is consumed
	// by the deferred refilter handler and invalidated on every new
	// projection.
	pendingListRestore pendingListRestore
	// reconcile stores the context needed to verify a disk mutation
	// (install/switch/delete) that reported a catalog error against a
	// fresh catalog fetch. It is zero/active=false when no
	// reconciliation is pending.
	reconcile reconcileContext

	list              list.Model
	installedTable    table.Model
	Loading           bool
	Spinner           spinner.Model
	CurrentTab        int
	InstallingVersion string
	// Status owns the status triplet (text, kind, scope) as the
	// StatusLine value-type module. Reads go through Text/Kind/Scope;
	// mutations go through SetTab/SetGlobal/Clear/ClearTab.
	Status StatusLine
	// ShimPathWarning is the pre-rendered PATH warning captured before
	// launching the TUI, so View does not resolve PATH on every render.
	ShimPathWarning  string
	ConfirmingDelete bool
	DeleteVersion    string
	Width            int
	Height           int
	TermWidth        int
	TermHeight       int
	Layout           styles.LayoutMode

	// theme is the immutable rendering snapshot used by View and every
	// renderer. main.go builds it once from settings at startup and
	// applyRuntimeTheme rebuilds it when the user toggles themes in the
	// Settings tab. It is the only source of styling for the model —
	// the styles package no longer carries any package-level state.
	theme styles.Theme

	// Deps groups every field and state machine flag related to the
	// "Deps" tab. Use the helpers in deps_state.go to keep the
	// main Model surface small.
	Deps     DepsState
	Settings SettingsState
}

// Theme returns the Model's current immutable theme snapshot. It is
// read-only access for tests and helpers that need to render outside
// of View(); production rendering goes through m.theme directly.
func (m Model) Theme() styles.Theme { return m.theme }

// New builds the top-level Model for the TUI. It owns the invariant
// setup that every caller needs: the spinner, the installed-versions
// and dependencies tables (columns + height + styles), the version
// list and its delegate, and the Deps/Settings sub-states.
//
// The theme parameter is the immutable styles.Theme value built by the
// caller (main.go at startup, applyRuntimeTheme on toggle, tests
// directly). Passing it as a parameter replaces the previous implicit
// "call ApplyTheme before New" contract — it is now impossible to
// construct a Model without its theme, which removes the fragile
// init() → ApplyTheme → New ordering that previously broke silently
// when a new dialog style was added.
func New(moduleDir, settingsPath string, settings config.Settings, shimPathWarning string, theme styles.Theme) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.SpinnerStyle

	installedTable := table.New(
		table.WithColumns(installedTableColumns(defaultConstructionWidth)),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	installedTable.SetStyles(tableStyles(theme))

	depTable := table.New(
		table.WithColumns(dependencyTableColumns(defaultConstructionWidth)),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	depTable.SetStyles(tableStyles(theme))

	delegate := listDefaultDelegate(theme)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Available Versions"
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)

	return Model{
		list:            l,
		catalog:         newVersionCatalog(theme),
		Spinner:         sp,
		Loading:         true,
		installedTable:  installedTable,
		Layout:          styles.LayoutNormal,
		theme:           theme,
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
	if m.list.Height() > 0 {
		return m.list.Height()
	}
	return 24
}

func (m Model) viewWidth() int {
	if m.Width > 0 {
		return m.Width
	}
	if m.list.Width() > 0 {
		return m.list.Width()
	}
	return 80
}

func (m Model) normalizedSettings() config.Settings {
	return config.Normalize(m.Settings.Values)
}
