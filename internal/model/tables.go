package model

import (
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/prune"
)

// Mark glyphs rendered in front of every module path: a filled circle
// for marked rows, an empty one otherwise, so the marked set reads as
// a checklist. Whether a marked module actually moves is decided by
// the update plan, not by the glyph.
const (
	markFilled = "● "
	markEmpty  = "○ "
)

func (s *depsTab) updateDependencyTable() {
	rows := make([]table.Row, 0, len(s.dependencies))
	paths := make([]string, 0, len(s.dependencies))
	listed := make(map[string]bool, len(s.marks))
	for _, d := range s.dependencies {
		if s.marks[d.Path] {
			listed[d.Path] = true
		}
		if s.display == config.DepsDisplayDirect && d.Indirect {
			continue
		}
		rows = append(rows, table.Row{markPrefix(*s, d) + d.Path, d.Version, d.Latest, dependencyStatus(d)})
		paths = append(paths, d.Path)
	}
	// Drop marks whose module disappeared from the list (e.g. after a
	// tidy removed it) so they cannot poison a later selection.
	for path := range s.marks {
		if !listed[path] {
			delete(s.marks, path)
		}
	}
	s.table.SetRows(rows)
	// The table parks its cursor at -1 once it has been given no
	// rows and never brings it back on its own; a listed table always
	// has the cursor on a row.
	if s.table.Cursor() < 0 && len(rows) > 0 {
		s.table.SetCursor(0)
	}
	s.rowPaths = paths
}

func markPrefix(state depsTab, d deps.ModuleDependency) string {
	if state.marked(d.Path) {
		return markFilled
	}
	return markEmpty
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
	versionWidth, sizeWidth, statusWidth, minPathWidth := 10, 10, 10, 18

	pathWidth := width - versionWidth - sizeWidth - statusWidth - 8
	if pathWidth < minPathWidth {
		pathWidth = minPathWidth
	}

	return []table.Column{
		{Title: "Version", Width: versionWidth},
		{Title: "Path", Width: pathWidth},
		{Title: "Size", Width: sizeWidth},
		{Title: "Status", Width: statusWidth},
	}
}

func formatDiskUsage(bytes int64) string {
	return prune.FormatBytes(bytes)
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
