package model

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// reconcileOperation identifies which disk mutation a reconciliation
// fetch is verifying.
type reconcileOperation int

const (
	reconcileInstall reconcileOperation = iota
	reconcileSwitch
	reconcileDelete
)

// reconcileContext is stored on the Model when a completion mutation
// (install/switch/delete) reports a catalog error. Because the disk
// operation may still have succeeded, the handler re-fetches the
// catalog and the resulting VersionsMsg uses this context to verify
// the expected side-effect before claiming success. For installs,
// warnings carries the non-fatal warnings reported by the install core
// so they survive the reconciliation fetch and surface in the final
// status.
type reconcileContext struct {
	active            bool
	operation         reconcileOperation
	version           string
	shimInPath        bool
	installWarnings   []install.Warning
	lifecycleWarnings []lifecycle.Warning
}

// pendingListRestore captures the Available-list selection that must be
// restored once the asynchronous refilter triggered by list.SetItems
// (while a filter is active) settles. It is consumed by the deferred
// refilter handler and invalidated on every new projection.
type pendingListRestore struct {
	active     bool
	generation uint64
	filterText string
	version    string
	index      int
}

// refilterSettledMsg is the private deferred wrapper around
// list.FilterMatchesMsg. Carrying the projection generation and filter
// text captured at wrap time lets the handler drop stale results after
// a newer projection or further filter input, and apply the matches
// together with the selection restore in a single coherent step.
type refilterSettledMsg struct {
	matches    list.FilterMatchesMsg
	generation uint64
	filterText string
}

// applyProjection is the private transactional helper: it applies one
// coherent catalog projection to both version widgets, bumping the
// projection generation and restoring the previously selected
// Available and Installed entries by identity (falling back to the old
// numeric index clamped to the new length). It returns the tea.Cmd
// produced by list.SetItems so callers can propagate the asynchronous
// refilter. Exactly one successful changed mutation maps to exactly
// one applyProjection call.
func (m *Model) applyProjection(p versionProjection) tea.Cmd {
	oldListVersion := m.selectedListVersion()
	oldListIndex := m.list.Index()
	if oldListVersion == "" && m.pendingListRestore.active && m.list.FilterState() != list.Unfiltered {
		oldListVersion = m.pendingListRestore.version
		oldListIndex = m.pendingListRestore.index
	}
	oldTableVersion, oldTableIndex := m.selectedInstalledIdentity()

	m.projectionGeneration++

	// The installed table is synchronous: set rows and restore the
	// selection in the same step.
	m.installedTable.SetRows(p.installed)
	m.restoreInstalledSelection(oldTableVersion, oldTableIndex)

	cmd := m.list.SetItems(p.available)

	// When no filter is active the list view is rebuilt synchronously
	// by SetItems, so the selection is restored now. When a filter is
	// active SetItems clears the filtered view and schedules a refilter
	// (FilterMatchesMsg); the restore must wait for that to settle, so
	// it is stashed for the deferred refilter handler.
	if m.list.FilterState() == list.Unfiltered {
		m.restoreListSelection(oldListVersion, oldListIndex)
		m.pendingListRestore = pendingListRestore{}
		return cmd
	}
	m.pendingListRestore = pendingListRestore{
		active:     true,
		generation: m.projectionGeneration,
		filterText: m.list.FilterInput.Value(),
		version:    oldListVersion,
		index:      oldListIndex,
	}
	return wrapRefilterCmd(cmd, m.projectionGeneration, m.pendingListRestore.filterText)
}

// replaceVersions is the single setup-facing Replace wrapper. It swaps
// the whole catalog; on a successful replace it rebuilds the
// projection, on error the previous catalog is preserved and the
// caller surfaces the failure.
func (m *Model) replaceVersions(versions []utils.GoVersion) (tea.Cmd, error) {
	changed, err := m.catalog.replace(versions)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return m.applyProjection(m.catalog.projection()), nil
}

func wrapRefilterCmd(cmd tea.Cmd, generation uint64, filterText string) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		matches, ok := msg.(list.FilterMatchesMsg)
		if !ok {
			return msg
		}
		return refilterSettledMsg{
			matches:    matches,
			generation: generation,
			filterText: filterText,
		}
	}
}

// markVersionInstalled, activateVersion and markVersionDeleted are the
// completion wrappers. Each delegates to the catalog and only when the
// catalog reports a real change does it apply a fresh projection.
func (m *Model) markVersionInstalled(version, path string) (bool, tea.Cmd, error) {
	changed, err := m.catalog.markInstalled(version, path)
	if err != nil {
		return changed, nil, err
	}
	if !changed {
		return false, nil, nil
	}
	return true, m.applyProjection(m.catalog.projection()), nil
}

func (m *Model) activateVersion(version string) (bool, tea.Cmd, error) {
	changed, err := m.catalog.activate(version)
	if err != nil {
		return changed, nil, err
	}
	if !changed {
		return false, nil, nil
	}
	return true, m.applyProjection(m.catalog.projection()), nil
}

func (m *Model) markVersionDeleted(version string) (bool, tea.Cmd, error) {
	changed, err := m.catalog.markDeleted(version)
	if err != nil {
		return changed, nil, err
	}
	if !changed {
		return false, nil, nil
	}
	return true, m.applyProjection(m.catalog.projection()), nil
}

// selectedListVersion returns the version string of the currently
// selected Available-list item, or "" when nothing is selected.
func (m Model) selectedListVersion() string {
	item := m.list.SelectedItem()
	if item == nil {
		return ""
	}
	si, ok := item.(styles.Item)
	if !ok {
		return ""
	}
	return si.Name
}

// selectedInstalledIdentity returns the version string and numeric
// cursor of the currently selected Installed-table row.
func (m Model) selectedInstalledIdentity() (string, int) {
	row := m.installedTable.SelectedRow()
	version := ""
	if len(row) > 0 {
		version = row[0]
	}
	return version, m.installedTable.Cursor()
}

// restoreListSelection re-selects the Available entry whose version
// matches version; if it is absent (or version is empty) it falls back
// to oldIndex clamped to the visible list length.
func (m *Model) restoreListSelection(version string, oldIndex int) {
	visible := m.list.VisibleItems()
	if len(visible) == 0 {
		return
	}
	target := -1
	if version != "" {
		for i, it := range visible {
			if si, ok := it.(styles.Item); ok && si.Name == version {
				target = i
				break
			}
		}
	}
	if target < 0 {
		target = oldIndex
	}
	if target < 0 {
		target = 0
	}
	if target >= len(visible) {
		target = len(visible) - 1
	}
	m.list.Select(target)
}

// restoreInstalledSelection re-selects the Installed row whose version
// matches version, falling back to oldIndex clamped to the row count.
func (m *Model) restoreInstalledSelection(version string, oldIndex int) {
	rows := m.installedTable.Rows()
	if len(rows) == 0 {
		return
	}
	target := -1
	if version != "" {
		for i, r := range rows {
			if len(r) > 0 && r[0] == version {
				target = i
				break
			}
		}
	}
	if target < 0 {
		target = oldIndex
	}
	if target < 0 {
		target = 0
	}
	if target >= len(rows) {
		target = len(rows) - 1
	}
	m.installedTable.SetCursor(target)
}

// verifyReconciliation checks the expected post-condition for the
// pending reconciliation operation against the freshly applied
// catalog snapshot.
func (m Model) verifyReconciliation() bool {
	if !m.reconcile.active {
		return false
	}
	v, ok := m.catalog.lookup(m.reconcile.version)
	if m.reconcile.operation == reconcileDelete {
		return !ok || !v.Installed
	}
	if !ok {
		return false
	}
	switch m.reconcile.operation {
	case reconcileInstall:
		return v.Installed && v.Path != ""
	case reconcileSwitch:
		return v.Active
	}
	return false
}

// applyReconciliationSuccess writes the success status for a verified
// reconciliation, preserving the scope and text the direct completion
// path would have used.
func (m *Model) applyReconciliationSuccess(rc reconcileContext) {
	m.Loading = false
	switch rc.operation {
	case reconcileInstall:
		text, kind := installSuccessStatus(rc.version, rc.installWarnings)
		m.Status.SetGlobal(text, kind)
	case reconcileSwitch:
		if len(rc.lifecycleWarnings) > 0 {
			m.Status.SetGlobal(fmt.Sprintf("Switched to Go %s with warnings: %s", rc.version, joinLifecycleWarnings(rc.lifecycleWarnings)), "warning")
			return
		}
		if rc.shimInPath {
			m.Status.SetTab(fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", rc.version), "success")
		} else {
			m.Status.SetTab(fmt.Sprintf("Switched to Go %s!\n\n%s", rc.version, utils.GetShimPathInstructions()), "success")
		}
	case reconcileDelete:
		if len(rc.lifecycleWarnings) > 0 {
			m.Status.SetGlobal(fmt.Sprintf("Deleted Go %s with warnings: %s", rc.version, joinLifecycleWarnings(rc.lifecycleWarnings)), "warning")
			return
		}
		m.Status.SetGlobal(fmt.Sprintf("Successfully deleted Go %s", rc.version), "success")
	}
}
