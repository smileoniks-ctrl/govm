package model

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	MessageType       string // "success" or "error"
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Handle dependency update confirmation modal first so it
		// captures input regardless of the active tab.
		if m.ConfirmingDependencyUpdate {
			return m.handleUpdateConfirmKey(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			// Switch between tabs
			m.CurrentTab = (m.CurrentTab + 1) % 3
			// Lazy-load deps on first visit
			if m.CurrentTab == 2 && !m.DependenciesLoaded {
				m.CheckingDependencies = true
				return m, utils.ListModuleDependencies(m.ModuleDir)
			}
			return m, nil
		case "i":
			if m.CurrentTab == 0 {
				selectedItem := m.List.SelectedItem().(styles.Item)
				for _, v := range m.Versions {
					if v.Version == selectedItem.Name && !v.Installed {
						m.Loading = true
						m.InstallingVersion = v.Version
						m.Message = ""
						return m, utils.DownloadAndInstall(v)
					}
				}
			}
		case "u":
			if m.CurrentTab == 0 {
				selectedItem := m.List.SelectedItem().(styles.Item)
				for _, v := range m.Versions {
					if v.Version == selectedItem.Name && v.Installed {
						m.Loading = true
						m.Message = fmt.Sprintf("Switching to Go %s...", v.Version)
						return m, utils.SwitchVersion(v)
					}
				}
				m.Message = "You need to install this version first. Press 'i' to install."
				m.MessageType = "error"
			} else if m.CurrentTab == 2 && m.DependenciesLoaded && !m.UpdatingDependencies {
				updatable := utils.UpdatableDirectDependencies(m.Dependencies)
				if len(updatable) == 0 {
					m.Message = "No direct dependency updates available."
					m.MessageType = "warning"
					return m, nil
				}
				m.ConfirmingDependencyUpdate = true
				m.UpdateChoiceYes = true
				m.Message = ""
				m.MessageType = ""
				return m, nil
			}
		case "r":
			if m.CurrentTab == 2 {
				m.CheckingDependencies = true
				m.Message = "Checking for dependency updates..."
				m.MessageType = "info"
				return m, utils.CheckModuleDependencyUpdates(m.ModuleDir)
			}
			m.Loading = true
			m.Message = ""
			return m, utils.FetchGoVersions
		case "d":
			if m.CurrentTab == 0 || m.CurrentTab == 1 {
				selectedItem := m.List.SelectedItem().(styles.Item)
				for _, v := range m.Versions {
					if v.Version == selectedItem.Name && v.Installed {
						if v.Active {
							m.Message = "Cannot delete active version. Switch to another version first."
							m.MessageType = "error"
							return m, nil
						}

						m.ConfirmingDelete = true
						m.DeleteVersion = v.Version
						m.Message = fmt.Sprintf("Are you sure you want to delete Go %s? Press Y to confirm, N to cancel.", v.Version)
						m.MessageType = "warning"
						return m, nil
					}
				}

				if m.CurrentTab == 0 {
					m.Message = "This version is not installed."
					m.MessageType = "error"
				}
			}
		case "y", "Y":
			if m.ConfirmingDelete {
				m.ConfirmingDelete = false
				m.Loading = true
				m.Message = fmt.Sprintf("Deleting Go %s...", m.DeleteVersion)
				m.MessageType = "info"

				var versionToDelete utils.GoVersion
				for _, v := range m.Versions {
					if v.Version == m.DeleteVersion {
						versionToDelete = v
						break
					}
				}

				return m, utils.DeleteVersion(versionToDelete)
			}
		case "n", "N":
			if m.ConfirmingDelete {
				m.ConfirmingDelete = false
				m.DeleteVersion = ""
				m.Message = "Delete operation canceled."
				m.MessageType = "info"
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		m.Layout = styles.GetLayoutMode(msg.Width)

		frameH, frameV := styles.FrameOverhead(m.Layout)
		contentWidth := msg.Width - frameH
		if contentWidth < styles.MinTermWidth {
			contentWidth = styles.MinTermWidth
		}

		const fixedUIElements = 6
		contentHeight := msg.Height - frameV - fixedUIElements
		if contentHeight < styles.MinTermHeight {
			contentHeight = styles.MinTermHeight
		}

		m.Width = contentWidth
		m.Height = contentHeight
		m.List.SetSize(contentWidth, contentHeight)
		m.InstalledTable.SetWidth(contentWidth)
		m.InstalledTable.SetHeight(contentHeight)
		m.InstalledTable.SetColumns(installedTableColumns(contentWidth, m.Layout))
		m.DependencyTable.SetWidth(contentWidth)
		m.DependencyTable.SetHeight(contentHeight)
		m.DependencyTable.SetColumns(dependencyTableColumns(contentWidth, m.Layout))
		return m, nil
	case utils.ErrMsg:
		m.Err = nil
		m.Loading = false
		m.Message = msg.Error()
		m.MessageType = "error"
		return m, nil
	case utils.VersionsMsg:
		m.Err = nil
		m.Versions = msg
		items := make([]list.Item, len(m.Versions))
		for i, v := range m.Versions {
			items[i] = styles.Item{
				Name:            v.Version,
				DescriptionText: "go" + v.Version + " " + v.Filename,
				Installed:       v.Installed,
				Active:          v.Active,
			}
		}
		m.List.SetItems(items)
		m.Loading = false
		m.updateInstalledTable()
		return m, nil
	case utils.DependenciesMsg:
		m.Dependencies = msg
		m.DependenciesLoaded = true
		m.CheckingDependencies = false
		m.updateDependencyTable()
		m.Message = ""
		m.MessageType = ""
		return m, nil
	case utils.DependenciesUpdatedMsg:
		m.UpdatingDependencies = false
		m.Dependencies = msg.Dependencies
		m.updateDependencyTable()
		m.Message = fmt.Sprintf("Updated %d direct %s", msg.Updated, pluralize(msg.Updated, "dependency", "dependencies"))
		m.MessageType = "success"
		return m, nil
	case utils.DependencyErrMsg:
		if m.UpdatingDependencies {
			m.UpdatingDependencies = false
		}
		if m.CheckingDependencies {
			m.CheckingDependencies = false
		}
		if msg.Err != nil {
			m.Message = msg.Err.Error()
		}
		m.MessageType = "error"
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd
	case utils.DownloadCompleteMsg:
		m.Loading = false
		m.InstallingVersion = ""
		for i, v := range m.Versions {
			if v.Version == msg.Version {
				m.Versions[i].Installed = true
				m.Versions[i].Path = msg.Path
				break
			}
		}
		items := m.List.Items()
		for i, it := range items {
			if it.(styles.Item).Name == msg.Version {
				updatedItem := it.(styles.Item)
				updatedItem.Installed = true
				items[i] = updatedItem
			}
		}
		m.List.SetItems(items)
		m.updateInstalledTable()
		m.Message = fmt.Sprintf("Successfully installed Go %s", msg.Version)
		m.MessageType = "success"
		return m, nil
	case utils.SwitchCompletedMsg:
		m.Loading = false
		for i := range m.Versions {
			m.Versions[i].Active = (m.Versions[i].Version == msg.Version)
		}
		items := m.List.Items()
		for i, it := range items {
			updatedItem := it.(styles.Item)
			updatedItem.Active = (updatedItem.Name == msg.Version)
			items[i] = updatedItem
		}
		m.List.SetItems(items)
		m.updateInstalledTable()
		if msg.ShimInPath {
			m.Message = fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", msg.Version)
		} else {
			m.Message = fmt.Sprintf("Switched to Go %s!\n\n%s",
				msg.Version, utils.GetShimPathInstructions())
		}
		m.MessageType = "success"
		return m, nil
	case utils.DeleteCompleteMsg:
		m.Loading = false

		for i, v := range m.Versions {
			if v.Version == msg.Version {
				m.Versions[i].Installed = false
				m.Versions[i].Path = ""
				break
			}
		}

		items := m.List.Items()
		for i, it := range items {
			if it.(styles.Item).Name == msg.Version {
				updatedItem := it.(styles.Item)
				updatedItem.Installed = false
				items[i] = updatedItem
			}
		}
		m.List.SetItems(items)

		m.updateInstalledTable()

		m.Message = fmt.Sprintf("Successfully deleted Go %s", msg.Version)
		m.MessageType = "success"
		return m, nil
	}
	newListModel, cmd := m.List.Update(msg)
	m.List = newListModel
	cmds = append(cmds, cmd)
	newTableModel, tableCmd := m.InstalledTable.Update(msg)
	m.InstalledTable = newTableModel
	cmds = append(cmds, tableCmd)
	newDepsTableModel, depsTableCmd := m.DependencyTable.Update(msg)
	m.DependencyTable = newDepsTableModel
	cmds = append(cmds, depsTableCmd)
	return m, tea.Batch(cmds...)
}

func (m *Model) updateInstalledTable() {
	rows := []table.Row{}
	for _, v := range m.Versions {
		if v.Installed {
			status := ""
			if v.Active {
				status = "active"
			}
			rows = append(rows, table.Row{v.Version, v.Path, status})
		}
	}
	m.InstalledTable.SetRows(rows)
}

func (m *Model) updateDependencyTable() {
	rows := []table.Row{}
	for _, d := range m.Dependencies {
		status := ""
		if d.Error != "" {
			status = "error"
		} else if d.Deprecated != "" {
			status = "deprecated"
		} else if d.Indirect && d.Latest != "" && d.Latest != d.Version {
			status = "indirect update"
		} else if d.Latest != "" && d.Latest != d.Version {
			status = "update avail"
		} else if d.Indirect {
			status = "indirect"
		} else {
			status = "current"
		}
		rows = append(rows, table.Row{d.Path, d.Version, d.Latest, status})
	}
	m.DependencyTable.SetRows(rows)
}

func (m Model) handleUpdateConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		m.UpdateChoiceYes = !m.UpdateChoiceYes
		return m, nil
	case "enter":
		return m.applyUpdateChoice()
	case "y", "Y", "д", "Д":
		m.UpdateChoiceYes = true
		return m.applyUpdateChoice()
	case "n", "N", "н", "Н", "esc":
		m.ConfirmingDependencyUpdate = false
		m.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}
	return m, nil
}

func (m Model) applyUpdateChoice() (tea.Model, tea.Cmd) {
	if !m.UpdateChoiceYes {
		m.ConfirmingDependencyUpdate = false
		m.UpdateChoiceYes = false
		m.Message = "Update canceled."
		m.MessageType = "info"
		return m, nil
	}

	m.ConfirmingDependencyUpdate = false
	m.UpdateChoiceYes = false
	m.UpdatingDependencies = true
	m.Message = "Updating dependencies..."
	m.MessageType = "info"
	return m, utils.UpdateModuleDependencies(m.ModuleDir, m.Dependencies)
}

func (m Model) View() tea.View {
	appStyle := styles.AppStyleFor(m.Layout)
	width := m.viewWidth()
	height := m.viewHeight()

	if m.Err != nil {
		return tea.NewView(appStyle.Render(renderStatus("error", fmt.Sprintf("Error: %s", m.Err), width)))
	}

	components := []string{
		renderHeader(width, m.Layout),
		renderTabs(m.CurrentTab, m.Layout),
	}

	if !utils.IsShimInPath() {
		instructions := utils.GetShimPathInstructions()
		components = append(components, renderStatus("warning", "GoVM is not in your PATH. "+instructions, width))
	}

	switch m.CurrentTab {
	case 0:
		components = append(components, m.List.View())
	case 1:
		components = append(components, m.InstalledTable.View())
	case 2:
		components = append(components, m.DependencyTable.View())
	}

	status := m.Message
	statusType := m.MessageType
	if m.Loading || m.CheckingDependencies || m.UpdatingDependencies {
		statusType = "info"
		if m.InstallingVersion != "" {
			status = fmt.Sprintf("%s Downloading Go %s", m.Spinner.View(), m.InstallingVersion)
		} else if status == "" {
			status = fmt.Sprintf("%s Loading", m.Spinner.View())
		}
	}
	if status != "" {
		components = append(components, renderStatus(statusType, status, width))
	}

	components = append(components, renderHelp(m.CurrentTab, m.ConfirmingDelete, m.ConfirmingDependencyUpdate, width, m.Layout))
	rendered := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, components...))

	if m.ConfirmingDependencyUpdate {
		rendered = overlayDialog(rendered, renderDependencyUpdateDialog(m.UpdateChoiceYes), width, height)
	}

	v := tea.NewView(rendered)
	v.AltScreen = true
	return v
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

func installedTableColumns(width int, layout styles.LayoutMode) []table.Column {
	var versionWidth, statusWidth, minWidth int
	switch layout {
	case styles.LayoutCompact:
		versionWidth, statusWidth, minWidth = 8, 6, 8
	default:
		versionWidth, statusWidth, minWidth = 10, 10, 18
	}

	pathWidth := width - versionWidth - statusWidth - 6
	if pathWidth < minWidth {
		pathWidth = minWidth
	}

	return []table.Column{
		{Title: "Version", Width: versionWidth},
		{Title: "Path", Width: pathWidth},
		{Title: "Status", Width: statusWidth},
	}
}

func dependencyTableColumns(width int, layout styles.LayoutMode) []table.Column {
	var pathWidth, versionWidth, latestWidth, statusWidth int
	switch layout {
	case styles.LayoutCompact:
		pathWidth, versionWidth, latestWidth, statusWidth = 12, 7, 7, 6
	default:
		pathWidth, versionWidth, latestWidth, statusWidth = 24, 9, 9, 10
	}

	// Adjust path column to fill remaining space
	used := versionWidth + latestWidth + statusWidth + 12
	pathWidth = width - used
	if pathWidth < 10 {
		pathWidth = 10
	}

	return []table.Column{
		{Title: "Dependency", Width: pathWidth},
		{Title: "Current", Width: versionWidth},
		{Title: "Latest", Width: latestWidth},
		{Title: "Status", Width: statusWidth},
	}
}

func renderHeader(width int, layout styles.LayoutMode) string {
	title := styles.TitleStyle.Render("GoVM")

	if layout == styles.LayoutCompact {
		return title
	}

	meta := styles.HeaderMetaStyle.Render("Go Version Manager")
	spacerWidth := maxInt(1, width-lipgloss.Width(title)-lipgloss.Width(meta))
	return lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", spacerWidth), meta)
}

func renderTabs(currentTab int, layout styles.LayoutMode) string {
	var availableLabel, installedLabel, depsLabel string
	if layout == styles.LayoutCompact {
		availableLabel, installedLabel, depsLabel = "All", "Local", "Deps"
	} else {
		availableLabel, installedLabel, depsLabel = "Available", "Installed", "Deps"
	}

	tabs := []string{
		renderTab(availableLabel, currentTab == 0),
		renderTab(installedLabel, currentTab == 1),
		renderTab(depsLabel, currentTab == 2),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
}

func renderTab(label string, active bool) string {
	if active {
		return styles.ActiveTabStyle.Render("● " + label)
	}
	return styles.InactiveTabStyle.Render("○ " + label)
}

func renderStatus(messageType, message string, width int) string {
	if message == "" {
		return ""
	}

	icon := "•"
	style := styles.StatusInfoStyle
	switch messageType {
	case "success":
		icon = "✓"
		style = styles.StatusSuccessStyle
	case "error":
		icon = "✕"
		style = styles.StatusErrorStyle
	case "warning":
		icon = "!"
		style = styles.StatusWarningStyle
	case "info":
		icon = "•"
		style = styles.StatusInfoStyle
	}

	return style.Width(width).Render(fmt.Sprintf("%s %s", icon, message))
}

func renderHelp(currentTab int, confirmingDelete bool, confirmingDeps bool, width int, layout styles.LayoutMode) string {
	var hints [][2]string

	if confirmingDeps {
		hints = [][2]string{
			{"←/→", "choose"},
			{"enter", "confirm"},
			{"esc", "cancel"},
			{"q", "quit"},
		}
		return renderKeyHints(hints, width, layout)
	}

	if confirmingDelete {
		hints = [][2]string{
			{"y", "confirm"},
			{"n", "cancel"},
			{"q", "quit"},
		}
	} else if currentTab == 0 {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"i", "inst"},
				{"u", "use"},
				{"d", "del"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"i", "install"},
				{"u", "use"},
				{"d", "delete"},
				{"r", "refresh"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	} else if currentTab == 2 {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"r", "check"},
				{"u", "update"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"r", "check updates"},
				{"u", "update"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	} else {
		if layout == styles.LayoutCompact {
			hints = [][2]string{
				{"u", "use"},
				{"d", "del"},
				{"tab", "sw"},
				{"q", "quit"},
			}
		} else {
			hints = [][2]string{
				{"u", "use"},
				{"d", "delete"},
				{"tab", "switch"},
				{"q", "quit"},
			}
		}
	}

	return renderKeyHints(hints, width, layout)
}

func renderKeyHints(hints [][2]string, width int, layout styles.LayoutMode) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, fmt.Sprintf("%s %s", styles.HelpKeyStyle.Render(hint[0]), styles.HelpTextStyle.Render(hint[1])))
	}

	helpText := strings.Join(parts, "  ")

	if layout == styles.LayoutCompact && lipgloss.Width(helpText) > width {
		separator := " "
		helpText = strings.Join(parts, separator)
	}

	if lipgloss.Width(helpText) > width {
		helpText = styles.TruncateText(helpText, width)
	}

	return helpText
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func renderDependencyUpdateDialog(yesSelected bool) string {
	warningStyle := styles.StatusWarningStyle.Bold(true)
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.Warning).
		Padding(0, 1)

	bodyStyle := lipgloss.NewStyle().
		Padding(0, 1)

	yesActive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(styles.Primary).
		Bold(true).
		Padding(0, 2)

	yesInactive := lipgloss.NewStyle().
		Foreground(styles.Muted).
		Padding(0, 2)

	noActive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(styles.Primary).
		Bold(true).
		Padding(0, 2)

	noInactive := lipgloss.NewStyle().
		Foreground(styles.Muted).
		Padding(0, 2)

	yesBtn, noBtn := yesInactive, noInactive
	if yesSelected {
		yesBtn, noBtn = yesActive, noInactive
	} else {
		yesBtn, noBtn = yesInactive, noActive
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Да"),
		"  ",
		noBtn.Render("Нет"),
	)

	dialog := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render(warningStyle.Render("⚠ Warning")),
		"",
		bodyStyle.Render("Вы уверены что хотите обновить зависимости?"),
		"",
		buttons,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Warning).
		Padding(1, 2).
		Width(54).
		Render(dialog)
}

func overlayDialog(background, dialog string, width, height int) string {
	dw := lipgloss.Width(dialog)
	dh := lipgloss.Height(dialog)

	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	if dw > width {
		dw = width
	}
	if dh > height {
		dh = height
	}

	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog,
		lipgloss.WithWhitespaceChars(" "),
	)

	bgLines := strings.Split(background, "\n")
	dialogLines := strings.Split(placed, "\n")

	// Center the dialog lines over the background lines.
	startRow := 0
	if len(bgLines) > len(dialogLines) {
		startRow = (len(bgLines) - len(dialogLines)) / 2
	}
	endRow := startRow + len(dialogLines)
	if endRow > len(bgLines) {
		endRow = len(bgLines)
	}
	dialogLines = dialogLines[:endRow-startRow]

	for i, dline := range dialogLines {
		row := startRow + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLine := bgLines[row]
		bgW := lipgloss.Width(bgLine)
		dW := lipgloss.Width(dline)
		col := 0
		if bgW > dW {
			col = (bgW - dW) / 2
		}
		bgLines[row] = spliceCentered(bgLine, dline, col)
	}

	return strings.Join(bgLines, "\n")
}

func spliceCentered(bg, overlay string, col int) string {
	bgRunes := []rune(bg)
	ovRunes := []rune(overlay)

	if col < 0 {
		col = 0
	}
	if col > len(bgRunes) {
		col = len(bgRunes)
	}

	prefix := string(bgRunes[:col])
	suffix := ""
	if col+len(ovRunes) < len(bgRunes) {
		suffix = string(bgRunes[col+len(ovRunes):])
	}
	return prefix + overlay + suffix
}
