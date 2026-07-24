package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/deps"
)

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
