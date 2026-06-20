package model

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/melkeydev/govm/internal/styles"
	"github.com/melkeydev/govm/internal/utils"
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
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			// Switch between tabs
			m.CurrentTab = (m.CurrentTab + 1) % 2
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
			}
		case "r":
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
		return m, nil
	case utils.ErrMsg:
		m.Err = msg
		m.Loading = false
		m.Message = msg.Error()
		m.MessageType = "error"
		return m, nil
	case utils.VersionsMsg:
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

func (m Model) View() tea.View {
	appStyle := styles.AppStyleFor(m.Layout)
	width := m.viewWidth()

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

	if m.CurrentTab == 0 {
		components = append(components, m.List.View())
	} else {
		components = append(components, m.InstalledTable.View())
	}

	status := m.Message
	statusType := m.MessageType
	if m.Loading {
		statusType = "info"
		if m.InstallingVersion != "" {
			status = fmt.Sprintf("%s Downloading Go %s", m.Spinner.View(), m.InstallingVersion)
		} else if status == "" {
			status = fmt.Sprintf("%s Loading versions", m.Spinner.View())
		}
	}
	if status != "" {
		components = append(components, renderStatus(statusType, status, width))
	}

	components = append(components, renderHelp(m.CurrentTab, m.ConfirmingDelete, width, m.Layout))
	v := tea.NewView(appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, components...)))
	v.AltScreen = true
	return v
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
	var availableLabel, installedLabel string
	if layout == styles.LayoutCompact {
		availableLabel, installedLabel = "All", "Local"
	} else {
		availableLabel, installedLabel = "Available", "Installed"
	}

	tabs := []string{
		renderTab(availableLabel, currentTab == 0),
		renderTab(installedLabel, currentTab == 1),
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

func renderHelp(currentTab int, confirmingDelete bool, width int, layout styles.LayoutMode) string {
	var hints [][2]string

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
