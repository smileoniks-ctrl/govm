package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type Model struct {
	List              list.Model
	Versions          []utils.GoVersion
	Err               error
	Loading           bool
	Spinner           spinner.Model
	HomeDir           string
	GoVersionsDir     string
	CurrentTab        int
	DownloadProgress  float64
	InstallingVersion string
	Message           string
	MessageType       string // "success", "error", "warning", or "info"
	InstalledTable    table.Model
	ConfirmingDelete  bool
	DeleteVersion     string
	Width             int
	Height            int
	TermWidth         int
	TermHeight        int
	Layout            styles.LayoutMode

	ModuleDir            string
	Dependencies         []utils.ModuleDependency
	DependencyTable      table.Model
	DependenciesLoaded   bool
	CheckingDependencies bool

	ConfirmingDependencyUpdate bool
	UpdateChoiceYes            bool
	UpdatingDependencies       bool
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
