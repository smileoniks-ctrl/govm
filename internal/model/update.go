package model

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := (&m).update(msg)
	if next, ok := updated.(*Model); ok {
		return *next, cmd
	}
	return updated, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.Settings.EditingDistributionSource {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			return m.handleDistributionSourceInputKey(key)
		}

		var cmd tea.Cmd
		m.Settings.DistributionSourceInput, cmd = m.Settings.DistributionSourceInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.Settings.EditingDepsBackupLimit {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			return m.handleDepsBackupLimitInputKey(key)
		}

		var cmd tea.Cmd
		m.Settings.DepsBackupLimitInput, cmd = m.Settings.DepsBackupLimitInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.Deps.Dialog.Active() {
			return m.handleDialogKey(msg)
		}
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		m.Layout = styles.GetLayoutMode(msg.Width)

		frameH, frameV := styles.FrameOverhead(m.Layout)
		contentWidth := msg.Width - frameH
		if contentWidth < 1 {
			contentWidth = 1
		}

		const fixedUIElements = 6
		contentHeight := msg.Height - frameV - fixedUIElements
		if contentHeight < 1 {
			contentHeight = 1
		}

		m.Width = contentWidth
		m.Height = contentHeight
		m.projection.resize(contentWidth, contentHeight)
		m.Deps.Table.SetWidth(contentWidth)
		m.Deps.Table.SetHeight(contentHeight)
		m.Deps.Table.SetColumns(dependencyTableColumns(contentWidth))
		return m, nil

	case catalogLoadedMsg:
		updated, cmd := m.handleCatalogOutcome(m.projection.acceptLoad(msg.RequestID, msg.Versions))
		usage := m.projection.setDiskUsage(m.DiskUsage.VersionBytes)
		return updated, tea.Batch(cmd, usage.cmd)

	case catalogLoadFailedMsg:
		return m.handleCatalogOutcome(m.projection.failLoad(msg.RequestID, msg.Err))

	case distributionSourceValidatedMsg:
		return m.handleDistributionSourceValidation(msg)

	case diskUsageMsg:
		m.DiskUsage = msg.Summary
		outcome := m.projection.setDiskUsage(msg.Summary.VersionBytes)
		if msg.Err != nil {
			m.Status.SetTab(fmt.Sprintf("Disk usage unavailable: %v", msg.Err), "warning")
		} else if len(msg.Summary.Warnings) > 0 {
			m.Status.SetTab("Disk usage is approximate; some files could not be inspected.", "warning")
		}
		return m, outcome.cmd

	case prunePreviewMsg:
		if !m.Prune.AcceptPreview(msg.Result) {
			if msg.Err != nil {
				m.Status.SetTab(fmt.Sprintf("Prune unavailable: %v", msg.Err), "error")
			} else {
				m.Status.SetTab("Nothing to prune.", "info")
			}
			return m, nil
		}
		if msg.Err != nil {
			m.Status.SetTab(fmt.Sprintf("Prune has warnings: %v", msg.Err), "warning")
		} else {
			m.Status.SetTab("Review the prune plan and press Y to confirm.", "warning")
		}
		return m, nil

	case pruneDoneMsg:
		m.Prune.Finish()
		if msg.Err != nil {
			m.Status.SetGlobal(fmt.Sprintf("Prune completed with warnings: %v", msg.Err), "warning")
		} else {
			m.Status.SetGlobal(
				fmt.Sprintf("Pruned %d object(s), freed %s.", len(msg.Result.Removed), formatDiskUsage(pruneResultBytes(msg.Result))),
				"success",
			)
		}
		outcome := m.projection.startLoad(catalogLoadPurposeRefresh)
		if outcome.kind != catalogProjectionOutcomeLoadStarted {
			return m, m.diskUsageCmd()
		}
		return m, tea.Batch(LoadVersionsCmd(m.loadCatalog, outcome.loadRequest), m.diskUsageCmd())

	case list.FilterMatchesMsg:
		return m, m.projection.updateAvailable(msg)

	case catalogProjectionRefilterMsg:
		return m, m.projection.settleRefilter(msg)

	case DependenciesMsg:
		m.Deps.Dependencies = msg
		m.Deps.Loaded = true
		m.Deps.Phase = OpIdle
		m.updateDependencyTable()
		m.Status.ClearTab()
		return m, nil

	case DependencyBackupsMsg:
		m.Deps.Phase = OpIdle
		m.Deps.Backups = msg
		if len(msg) == 0 {
			m.Status.SetTab("No dependency backups found.", "warning")
			return m, nil
		}
		m.Deps.Dialog = ConfirmDialog{
			Kind:      DialogRestore,
			ChoiceYes: true,
			MaxCursor: len(msg) - 1,
		}
		m.Status.SetTab("Select a dependency backup to restore.", "info")
		return m, nil

	case deps.CheckUpdatesDoneEvent,
		deps.ApplyUpdatesDoneEvent,
		deps.CompensateDoneEvent,
		deps.ChecksDoneEvent,
		deps.RollbackDoneEvent:
		return m.handleCycleEvent(msg.(deps.Event))

	case DependenciesRestoredMsg:
		m.setUpdatedDependencies(msg.Dependencies)
		m.Status.SetGlobal(fmt.Sprintf("Restored dependencies from %s.", msg.BackupName), "success")
		return m, nil

	case dependencyExecutionErrMsg:
		m.Deps.Cycle = deps.NewUpdateCycle()
		m.resetDialog()
		m.Status.SetGlobal(msg.Err.Error(), "error")
		return m, nil

	case DependencyErrMsg:
		m.Deps.Reset()
		if msg.Err != nil {
			m.Status.SetGlobal(msg.Err.Error(), "error")
			return m, nil
		}
		m.Status.SetGlobal("", "error")
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case installProgressMsg:
		return m, m.handleInstallProgress(msg)

	case installProgressPollMsg:
		return m, m.handleInstallProgressPoll(msg)

	case installSuccessMsg:
		return m.handleInstallSuccess(msg)

	case installFailureMsg:
		return m.handleInstallFailure(msg)

	case activationSuccessMsg:
		return m.handleActivationSuccess(msg)

	case deletionSuccessMsg:
		return m.handleDeletionSuccess(msg)

	case lifecycleFailureMsg:
		outcome := m.projection.failMutation(msg.OperationID, msg.Err)
		if outcome.kind == catalogProjectionOutcomeStale {
			return m, nil
		}
		m.Status.SetGlobal(fmt.Sprintf("Failed to %s Go %s: %v", msg.Operation, msg.Version, msg.Err), "error")
		return m, nil
	}

	cmds = append(cmds, m.projection.update(msg))
	newDepsTableModel, depsTableCmd := m.Deps.Table.Update(msg)
	m.Deps.Table = newDepsTableModel
	cmds = append(cmds, depsTableCmd)
	return m, tea.Batch(cmds...)
}

func pruneResultBytes(result prune.Result) int64 {
	var total int64
	for _, candidate := range result.Removed {
		total += candidate.Bytes
	}
	return total
}

func (m *Model) handleCatalogOutcome(outcome catalogProjectionOutcome) (tea.Model, tea.Cmd) {
	switch outcome.kind {
	case catalogProjectionOutcomeStale, catalogProjectionOutcomeSuppressed:
		return m, nil
	case catalogProjectionOutcomeLoadStarted:
		m.Status.SetGlobal(m.verifyingStatus(outcome.receipt.operation), "warning")
		return m, LoadVersionsCmd(m.loadCatalog, outcome.loadRequest)
	case catalogProjectionOutcomeReconciled:
		m.applyCompletion(outcome.receipt.operation)
		return m, outcome.cmd
	case catalogProjectionOutcomeRejected:
		if outcome.receipt.operation.id != 0 {
			m.Status.SetGlobal(fmt.Sprintf("Could not verify the operation: %v.", outcome.err), "error")
		} else {
			m.Status.SetGlobal(fmt.Sprintf("Failed to load Go versions: %v.", outcome.err), "error")
		}
		return m, outcome.cmd
	case catalogProjectionOutcomeFailed:
		if outcome.receipt.operation.id != 0 {
			m.Status.SetGlobal("The operation could not be confirmed against the installed catalog.", "error")
		} else {
			text := "catalog load failed"
			if outcome.err != nil {
				text = outcome.err.Error()
			}
			m.Status.SetGlobal(text, "error")
		}
		return m, outcome.cmd
	case catalogProjectionOutcomeCommittedWarning:
		m.applyCommittedProjectionWarning(outcome.receipt.operation, outcome.err)
		return m, outcome.cmd
	default:
		return m, outcome.cmd
	}
}

func (m *Model) handleInstallSuccess(msg installSuccessMsg) (tea.Model, tea.Cmd) {
	m.clearInstallProgress(msg.OperationID)
	outcome := m.projection.completeInstall(
		msg.OperationID,
		msg.Version,
		msg.Path,
		msg.Warnings,
	)
	updated, cmd := m.handleMutationCompletion(outcome)
	return updated, tea.Batch(cmd, m.diskUsageCmd())
}

func (m *Model) handleInstallFailure(msg installFailureMsg) (tea.Model, tea.Cmd) {
	m.clearInstallProgress(msg.OperationID)
	outcome := m.projection.failMutation(msg.OperationID, msg.Err)
	if outcome.kind == catalogProjectionOutcomeStale {
		return m, nil
	}
	m.Status.SetGlobal(fmt.Sprintf("Failed to install Go %s: %v", msg.Version, msg.Err), "error")
	return m, nil
}

func (m *Model) handleActivationSuccess(msg activationSuccessMsg) (tea.Model, tea.Cmd) {
	outcome := m.projection.completeActivation(
		msg.OperationID,
		msg.Result.Version,
		msg.Result.Warnings,
		msg.ShimInPath,
	)
	return m.handleMutationCompletion(outcome)
}

func (m *Model) handleDeletionSuccess(msg deletionSuccessMsg) (tea.Model, tea.Cmd) {
	outcome := m.projection.completeDeletion(
		msg.OperationID,
		msg.Result.Version,
		msg.Result.Warnings,
	)
	return m.handleMutationCompletion(outcome)
}

func (m *Model) handleMutationCompletion(outcome catalogProjectionOutcome) (tea.Model, tea.Cmd) {
	if outcome.kind == catalogProjectionOutcomeStale {
		return m, nil
	}
	if outcome.kind == catalogProjectionOutcomeLoadStarted {
		return m.handleCatalogOutcome(outcome)
	}
	if outcome.kind == catalogProjectionOutcomeCommittedWarning {
		return m.handleCatalogOutcome(outcome)
	}
	if outcome.kind == catalogProjectionOutcomePublished || outcome.kind == catalogProjectionOutcomeNoop {
		m.applyCompletion(outcome.receipt.operation)
	}
	return m, outcome.cmd
}

func (m *Model) applyCompletion(operation catalogOperation) {
	switch operation.kind {
	case catalogMutationInstall:
		text, kind := installSuccessStatus(operation.version, operation.installWarnings)
		m.Status.SetGlobal(text, kind)
	case catalogMutationActivation:
		if len(operation.lifecycleWarnings) > 0 {
			m.Status.SetGlobal(
				fmt.Sprintf(
					"Switched to Go %s with warnings: %s",
					operation.version,
					joinLifecycleWarnings(operation.lifecycleWarnings),
				),
				"warning",
			)
			return
		}
		if operation.shimInPath {
			m.Status.SetTab(
				fmt.Sprintf("Switched to Go %s! Run 'go version' to verify.", operation.version),
				"success",
			)
			return
		}
		m.Status.SetTab(
			fmt.Sprintf("Switched to Go %s!\n\n%s", operation.version, utils.GetShimPathInstructions()),
			"success",
		)
	case catalogMutationDeletion:
		if len(operation.lifecycleWarnings) > 0 {
			m.Status.SetGlobal(
				fmt.Sprintf(
					"Deleted Go %s with warnings: %s",
					operation.version,
					joinLifecycleWarnings(operation.lifecycleWarnings),
				),
				"warning",
			)
			return
		}
		m.Status.SetGlobal(fmt.Sprintf("Successfully deleted Go %s", operation.version), "success")
	}
}

func (m *Model) applyCommittedProjectionWarning(operation catalogOperation, err error) {
	action := "Version operation"
	switch operation.kind {
	case catalogMutationInstall:
		action = fmt.Sprintf("Installed Go %s", operation.version)
	case catalogMutationActivation:
		action = fmt.Sprintf("Switched to Go %s", operation.version)
	case catalogMutationDeletion:
		action = fmt.Sprintf("Deleted Go %s", operation.version)
	}
	m.Status.SetGlobal(
		fmt.Sprintf("%s, but the catalog view could not be updated: %v. Refresh to synchronize.", action, err),
		"warning",
	)
}

func (m Model) verifyingStatus(operation catalogOperation) string {
	if operation.id == 0 {
		return "Verifying catalog..."
	}
	switch operation.kind {
	case catalogMutationInstall:
		return fmt.Sprintf("Installed Go %s; verifying catalog...", operation.version)
	case catalogMutationActivation:
		return fmt.Sprintf("Switched to Go %s; verifying catalog...", operation.version)
	case catalogMutationDeletion:
		return fmt.Sprintf("Deleted Go %s; verifying catalog...", operation.version)
	default:
		return "Verifying catalog..."
	}
}
