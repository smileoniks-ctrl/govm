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

	// LastDependencySnapshot holds the pre-update state of go.mod
	// and go.sum together with the list of modules that were
	// upgraded. It is populated by DependenciesUpdatedMsg and
	// consumed by RollbackModuleDependencies.
	LastDependencySnapshot *utils.DependencySnapshot

	// ConfirmingDependencyChecks is true when the user is being
	// asked whether to run post-update checks.
	ConfirmingDependencyChecks bool
	// CheckChoiceYes tracks the current toggle in the checks dialog.
	// Yes means "run checks now", No means "skip them".
	CheckChoiceYes bool
	// RunningDependencyChecks is true while go test / go vet run.
	RunningDependencyChecks bool

	// ConfirmingDependencyRollback is true when the user is being
	// asked whether to roll back after a failed check.
	ConfirmingDependencyRollback bool
	// RollbackChoiceYes tracks the current toggle in the rollback
	// dialog. Yes means "restore the snapshot", No means "keep the
	// updated dependencies as is".
	RollbackChoiceYes bool
	// RollingBackDependencies is true while the module files are
	// being restored from the snapshot.
	RollingBackDependencies bool

	// LastCheckResult retains the most recent DependencyCheckResultMsg
	// so the rollback dialog can show what failed.
	LastCheckResult *utils.DependencyCheckResultMsg
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
