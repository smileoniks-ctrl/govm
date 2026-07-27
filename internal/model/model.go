package model

import (
	"context"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/application"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

type changeDistributionSourceFunc func(context.Context, string) (application.DistributionSourceResult, error)

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
	projection  catalogProjectionAdapter
	initialLoad catalogLoadRequest

	Spinner    spinner.Model
	Progress   progress.Model
	CurrentTab int
	// Status owns the status triplet (text, kind, scope) as the
	// StatusLine value-type module. Reads go through Text/Kind/Scope;
	// mutations go through SetTab/SetGlobal/Clear/ClearTab.
	Status StatusLine
	// ShimPathWarning is the pre-rendered PATH warning captured before
	// launching the TUI, so View does not resolve PATH on every render.
	ShimPathWarning  string
	ConfirmingDelete bool
	DeleteVersion    string
	// Prune owns the prune flow (phase plus the plan awaiting
	// confirmation) as the PruneState value-type module.
	Prune      PruneState
	DiskUsage  prune.Summary
	Width      int
	Height     int
	TermWidth  int
	TermHeight int
	Layout     styles.LayoutMode

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

	loadCatalog         loadCatalogFunc
	distributionSource  changeDistributionSourceFunc
	installGo           installFunc
	installWithProgress installProgressFunc
	activateGo          activateFunc
	deleteGo            deleteFunc
	previewPrune        previewPruneFunc
	runPrune            pruneFunc
	diskUsage           diskUsageFunc
	shimInPath          func() bool
}

type programModel struct {
	model          Model
	lastRefreshKey time.Time
}

const refreshKeyRepeatWindow = 750 * time.Millisecond

func NewProgramModel(model Model) tea.Model {
	return newProgramModel(model)
}

func FilterProgramMessage(current tea.Model, msg tea.Msg) tea.Msg {
	program, ok := current.(*programModel)
	if !ok {
		return msg
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || key.String() != "r" {
		return msg
	}
	now := time.Now()
	repeated := key.IsRepeat ||
		(!program.lastRefreshKey.IsZero() && now.Sub(program.lastRefreshKey) < refreshKeyRepeatWindow)
	program.lastRefreshKey = now
	if repeated || program.model.refreshInFlight() {
		return nil
	}
	return msg
}

func newProgramModel(model Model) *programModel {
	return &programModel{model: model}
}

func (m *programModel) Init() tea.Cmd {
	return m.model.Init()
}

func (m *programModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.model.update(msg)
	return m, cmd
}

func (m *programModel) View() tea.View {
	return m.model.View()
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

	depTable := table.New(
		table.WithColumns(dependencyTableColumns(defaultConstructionWidth)),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	depTable.SetStyles(tableStyles(theme))

	projection := newCatalogProjectionAdapter(theme)
	initialLoad := projection.startLoad(catalogLoadPurposeInitial).loadRequest

	return Model{
		projection:      projection,
		initialLoad:     initialLoad,
		Spinner:         sp,
		Progress:        newInstallProgressModel(theme),
		Layout:          styles.LayoutNormal,
		theme:           theme,
		Deps:            NewDepsState(moduleDir, depTable),
		Settings:        NewSettingsState(settingsPath, settings),
		ShimPathWarning: shimPathWarning,
	}
}

// VersionOperations contains the narrow process-composed seams used by the
// TUI for installed-version mutations and PATH presentation.
type VersionOperations struct {
	LoadCatalog         loadCatalogFunc
	DistributionSource  changeDistributionSourceFunc
	Install             installFunc
	InstallWithProgress installProgressFunc
	Activate            activateFunc
	Delete              deleteFunc
	PreviewPrune        previewPruneFunc
	Prune               pruneFunc
	DiskUsage           diskUsageFunc
	ShimInPath          func() bool
}

// BindVersionOperations returns a copy of m bound to process-wide services.
func (m Model) BindVersionOperations(operations VersionOperations) Model {
	m.loadCatalog = operations.LoadCatalog
	m.distributionSource = operations.DistributionSource
	m.installGo = operations.Install
	m.installWithProgress = operations.InstallWithProgress
	m.activateGo = operations.Activate
	m.deleteGo = operations.Delete
	m.previewPrune = operations.PreviewPrune
	m.runPrune = operations.Prune
	m.diskUsage = operations.DiskUsage
	m.shimInPath = operations.ShimInPath
	return m
}

func (m Model) Init() tea.Cmd {
	var load tea.Cmd
	if m.initialLoad.ID != 0 {
		load = LoadVersionsCmd(m.loadCatalog, m.initialLoad)
	}
	var usage tea.Cmd
	if m.diskUsage != nil {
		usage = m.diskUsageCmd()
	}
	return tea.Batch(
		load,
		usage,
		m.Spinner.Tick,
	)
}

func (m Model) viewHeight() int {
	if m.Height > 0 {
		return m.Height
	}
	available := m.projection.availableModel()
	if available.Height() > 0 {
		return available.Height()
	}
	return 24
}

func (m Model) viewWidth() int {
	if m.Width > 0 {
		return m.Width
	}
	available := m.projection.availableModel()
	if available.Width() > 0 {
		return available.Width()
	}
	return 80
}

func (m Model) normalizedSettings() config.Settings {
	return config.Normalize(m.Settings.Values)
}
