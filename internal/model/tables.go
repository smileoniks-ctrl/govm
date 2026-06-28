package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m *Model) updateInstalledTable() {
	installed := 0
	for _, v := range m.Versions {
		if v.Installed {
			installed++
		}
	}
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
	for _, d := range m.Deps.Dependencies {
		rows = append(rows, table.Row{d.Path, d.Version, d.Latest, dependencyStatus(d)})
	}
	m.Deps.Table.SetRows(rows)
}

// dependencyStatus returns a short status string for a module dependency
// describing its update state. Priority order is intentional:
// error > deprecated > indirect update > update avail > indirect > current.
func dependencyStatus(d utils.ModuleDependency) string {
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

func installedTableColumns(width int, layout styles.LayoutMode) []table.Column {
	var versionWidth, statusWidth, minPathWidth int
	switch layout {
	case styles.LayoutCompact:
		versionWidth, statusWidth, minPathWidth = 8, 6, 8
	default:
		versionWidth, statusWidth, minPathWidth = 10, 10, 18
	}

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

func dependencyTableColumns(width int, layout styles.LayoutMode) []table.Column {
	var pathWidth, versionWidth, latestWidth, statusWidth, minPathWidth int
	switch layout {
	case styles.LayoutCompact:
		pathWidth, versionWidth, latestWidth, statusWidth, minPathWidth = 12, 7, 7, 6, 10
	default:
		pathWidth, versionWidth, latestWidth, statusWidth, minPathWidth = 24, 9, 9, 10, 10
	}

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
