package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// rebuildVersionViews is the single owner of the two derived views of
// m.Versions: the Available Versions list (m.List) and the Installed
// Versions table (m.InstalledTable). Every msg-handler that mutates
// m.Versions must call this afterwards so that the list, the table and
// the source slice never diverge. A nil/empty m.Versions yields an
// empty list and an empty table.
//
// Item titles are pre-rendered here through styles.RenderItemTitle so
// the per-frame list.View hot path performs only field reads instead
// of N lipgloss.Style.Render calls. The theme used is m.theme, which
// applyRuntimeTheme keeps in sync with the user's current theme.
func (m *Model) rebuildVersionViews() {
	t := m.theme
	items := make([]list.Item, len(m.Versions))
	installed := 0
	for i, v := range m.Versions {
		items[i] = styles.Item{
			Name:            v.Version,
			DescriptionText: v.DisplayDescription(),
			RenderedTitle:   styles.RenderItemTitle(t, v.Version, v.Installed, v.Active),
		}
		if v.Installed {
			installed++
		}
	}
	m.List.SetItems(items)

	rows := make([]table.Row, 0, installed)
	for _, v := range m.Versions {
		if !v.Installed {
			continue
		}
		status := ""
		if v.Active {
			status = "active"
		}
		rows = append(rows, table.Row{v.Version, v.Path, status})
	}
	m.InstalledTable.SetRows(rows)
}

func (m *Model) updateDependencyTable() {
	rows := make([]table.Row, 0, len(m.Deps.Dependencies))
	settings := m.normalizedSettings()
	for _, d := range m.Deps.Dependencies {
		if settings.DepsDisplay == config.DepsDisplayDirect && d.Indirect {
			continue
		}
		rows = append(rows, table.Row{d.Path, d.Version, d.Latest, dependencyStatus(d)})
	}
	m.Deps.Table.SetRows(rows)
}

// dependencyStatus returns a short status string for a module dependency
// describing its update state. Priority order is intentional:
// error > deprecated > indirect update > update avail > indirect > current.
func dependencyStatus(d deps.ModuleDependency) string {
	switch {
	case d.Error != "":
		return "error"
	case d.Deprecated != "":
		return "deprecated"
	case d.Indirect && d.Latest != "" && d.Latest != d.Version:
		return "indirect update"
	case d.Latest != "" && d.Latest != d.Version:
		return "update avail"
	case d.Indirect:
		return "indirect"
	default:
		return "current"
	}
}

func installedTableColumns(width int) []table.Column {
	versionWidth, statusWidth, minPathWidth := 10, 10, 18

	pathWidth := width - versionWidth - statusWidth - 6
	if pathWidth < minPathWidth {
		pathWidth = minPathWidth
	}

	return []table.Column{
		{Title: "Version", Width: versionWidth},
		{Title: "Path", Width: pathWidth},
		{Title: "Status", Width: statusWidth},
	}
}

func dependencyTableColumns(width int) []table.Column {
	pathWidth, versionWidth, latestWidth, statusWidth, minPathWidth := 0, 9, 9, 10, 10

	used := versionWidth + latestWidth + statusWidth + 12
	pathWidth = width - used
	if pathWidth < minPathWidth {
		pathWidth = minPathWidth
	}

	return []table.Column{
		{Title: "Dependency", Width: pathWidth},
		{Title: "Current", Width: versionWidth},
		{Title: "Latest", Width: latestWidth},
		{Title: "Status", Width: statusWidth},
	}
}
