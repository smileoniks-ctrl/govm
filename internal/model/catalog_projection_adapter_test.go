package model

import (
	"errors"
	"reflect"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/state"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func newCatalogProjectionAdapterTestFixture(t *testing.T, versions []utils.GoVersion) catalogProjectionAdapter {
	t.Helper()

	adapter := newCatalogProjectionAdapter(testTheme())
	outcome := adapter.replaceSnapshot(versions)
	if outcome.kind == catalogProjectionOutcomeRejected {
		t.Fatalf("replaceSnapshot() error = %v", outcome.err)
	}
	return adapter
}

func TestCatalogProjectionAdapterMutationRegistryValueCopyIsolation(t *testing.T) {
	tests := []struct {
		name       string
		changeCopy func(*catalogProjectionAdapter, catalogOperation) catalogProjectionOutcome
		wantKind   catalogProjectionOutcomeKind
	}{
		{
			name: "completion in copy",
			changeCopy: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeInstall(operation.id, operation.version, "/copy", nil)
			},
			wantKind: catalogProjectionOutcomePublished,
		},
		{
			name: "failure in copy",
			changeCopy: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.failMutation(operation.id, errors.New("copy failed"))
			},
			wantKind: catalogProjectionOutcomeFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := newCatalogProjectionAdapterTestFixture(t, []utils.GoVersion{{Version: "1.30.0"}})
			operation := original.startMutation(catalogMutationInstall, "1.30.0")
			copied := original

			if outcome := tt.changeCopy(&copied, operation); outcome.kind != tt.wantKind {
				t.Fatalf("copy outcome kind = %v, want %v", outcome.kind, tt.wantKind)
			}
			if activity := original.activityState(); activity != (catalogActivity{
				kind:    catalogActivityInstalling,
				version: "1.30.0",
			}) {
				t.Fatalf("original activity = %+v, want installing 1.30.0", activity)
			}

			outcome := original.completeInstall(operation.id, "1.30.0", "/original", nil)
			if outcome.kind != catalogProjectionOutcomePublished {
				t.Fatalf("original completion kind = %v, want Published", outcome.kind)
			}
			got, ok := original.lookup("1.30.0")
			if !ok || !got.Installed || got.Path != "/original" {
				t.Fatalf("original lookup = %+v, found=%v, want installed at /original", got, ok)
			}
		})
	}
}

func TestCatalogProjectionAdapterLoadRegistryValueCopyIsolation(t *testing.T) {
	tests := []struct {
		name           string
		changeCopy     func(*catalogProjectionAdapter, uint64) catalogProjectionOutcome
		changeOriginal func(*catalogProjectionAdapter, uint64) catalogProjectionOutcome
		wantCopy       catalogProjectionOutcomeKind
		wantOriginal   catalogProjectionOutcomeKind
	}{
		{
			name: "accepting in copy",
			changeCopy: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.acceptLoad(requestID, []utils.GoVersion{{Version: "1.30.0"}})
			},
			changeOriginal: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.acceptLoad(requestID, []utils.GoVersion{{Version: "1.29.0"}})
			},
			wantCopy:     catalogProjectionOutcomePublished,
			wantOriginal: catalogProjectionOutcomePublished,
		},
		{
			name: "failing in copy",
			changeCopy: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.failLoad(requestID, errors.New("copy load failed"))
			},
			changeOriginal: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.failLoad(requestID, errors.New("original load failed"))
			},
			wantCopy:     catalogProjectionOutcomeFailed,
			wantOriginal: catalogProjectionOutcomeFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := newCatalogProjectionAdapter(testTheme())
			load := original.startLoad(catalogLoadPurposeRefresh)
			copied := original

			if outcome := tt.changeCopy(&copied, load.loadRequest.ID); outcome.kind != tt.wantCopy {
				t.Fatalf("copy outcome kind = %v, want %v", outcome.kind, tt.wantCopy)
			}
			if phase := original.operationPhase(); phase != catalogOperationPhaseLoading {
				t.Fatalf("original phase after copy transition = %v, want Loading", phase)
			}
			if outcome := tt.changeOriginal(&original, load.loadRequest.ID); outcome.kind != tt.wantOriginal {
				t.Fatalf("original outcome kind = %v, want %v", outcome.kind, tt.wantOriginal)
			}
		})
	}
}

func TestCatalogProjectionAdapterRefreshSuccessDuringMutation(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())
	load := adapter.startLoad(catalogLoadPurposeRefresh)
	operation := adapter.startMutation(catalogMutationInstall, "1.30.0")

	outcome := adapter.acceptLoad(load.loadRequest.ID, []utils.GoVersion{{Version: "1.30.0"}})
	if outcome.kind != catalogProjectionOutcomePublished {
		t.Fatalf("refresh outcome kind = %v, want Published", outcome.kind)
	}
	if phase := adapter.operationPhase(); phase != catalogOperationPhaseMutating {
		t.Fatalf("phase = %v, want Mutating", phase)
	}
	if activity := adapter.activityState(); activity != (catalogActivity{
		kind:    catalogActivityInstalling,
		version: "1.30.0",
	}) {
		t.Fatalf("activity = %+v, want installing 1.30.0", activity)
	}
	if second := adapter.startMutation(catalogMutationDeletion, "1.29.0"); second.id != 0 {
		t.Fatalf("second operation = %+v, want zero operation", second)
	}

	outcome = adapter.completeInstall(operation.id, operation.version, "/p/1.30.0", nil)
	if outcome.kind != catalogProjectionOutcomePublished {
		t.Fatalf("mutation outcome kind = %v, want Published", outcome.kind)
	}
	got, ok := adapter.lookup("1.30.0")
	if !ok || !got.Installed || got.Path != "/p/1.30.0" {
		t.Fatalf("lookup = %+v, found=%v, want installed at /p/1.30.0", got, ok)
	}
}

func TestCatalogProjectionAdapterRefreshFailureDuringMutationIsSuppressed(t *testing.T) {
	adapter := newCatalogProjectionAdapterTestFixture(t, []utils.GoVersion{{Version: "1.29.0"}})
	load := adapter.startLoad(catalogLoadPurposeRefresh)
	adapter.startMutation(catalogMutationInstall, "1.29.0")

	loadErr := errors.New("refresh failed")
	outcome := adapter.failLoad(load.loadRequest.ID, loadErr)
	if outcome.kind != catalogProjectionOutcomeSuppressed || !errors.Is(outcome.err, loadErr) {
		t.Fatalf("failure outcome = %+v, want Suppressed with refresh error", outcome)
	}
	if phase := adapter.operationPhase(); phase != catalogOperationPhaseMutating {
		t.Fatalf("phase = %v, want Mutating", phase)
	}
	if activity := adapter.activityState(); activity != (catalogActivity{
		kind:    catalogActivityInstalling,
		version: "1.29.0",
	}) {
		t.Fatalf("activity = %+v, want installing 1.29.0", activity)
	}
	got, ok := adapter.lookup("1.29.0")
	if !ok || got.Installed {
		t.Fatalf("lookup after suppressed refresh failure = %+v, found=%v, want prior projection", got, ok)
	}
}

func TestCatalogProjectionAdapterMutationInvalidatesOlderLoad(t *testing.T) {
	tests := []struct {
		name    string
		respond func(*catalogProjectionAdapter, uint64) catalogProjectionOutcome
	}{
		{
			name: "older success",
			respond: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.acceptLoad(requestID, []utils.GoVersion{{
					Version:   "1.30.0",
					Installed: false,
				}})
			},
		},
		{
			name: "older failure",
			respond: func(adapter *catalogProjectionAdapter, requestID uint64) catalogProjectionOutcome {
				return adapter.failLoad(requestID, errors.New("late refresh failure"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapterTestFixture(t, []utils.GoVersion{{Version: "1.30.0"}})
			load := adapter.startLoad(catalogLoadPurposeRefresh)
			operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
			completed := adapter.completeInstall(operation.id, operation.version, "/direct", nil)
			if completed.kind != catalogProjectionOutcomePublished {
				t.Fatalf("completion kind = %v, want Published", completed.kind)
			}

			if outcome := tt.respond(&adapter, load.loadRequest.ID); outcome.kind != catalogProjectionOutcomeStale {
				t.Fatalf("older load outcome kind = %v, want Stale", outcome.kind)
			}
			got, ok := adapter.lookup("1.30.0")
			if !ok || !got.Installed || got.Path != "/direct" {
				t.Fatalf("lookup after stale response = %+v, found=%v, want direct mutation result", got, ok)
			}
		})
	}
}

func TestCatalogProjectionAdapterReconciliationReceipt(t *testing.T) {
	t.Run("install warnings are copied", func(t *testing.T) {
		adapter := newCatalogProjectionAdapter(testTheme())
		operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
		warnings := []install.Warning{{Kind: install.WarningCleanup, Err: errors.New("cleanup failed")}}
		started := adapter.completeInstall(operation.id, operation.version, "/p/1.30.0", warnings)
		if started.kind != catalogProjectionOutcomeLoadStarted {
			t.Fatalf("completion kind = %v, want LoadStarted", started.kind)
		}
		warnings[0] = install.Warning{Kind: install.WarningIntegrityUnavailable}

		other := adapter.startLoad(catalogLoadPurposeRefresh)
		if outcome := adapter.acceptLoad(other.loadRequest.ID, nil); outcome.kind != catalogProjectionOutcomeStale {
			t.Fatalf("non-verification load outcome = %v, want Stale", outcome.kind)
		}
		outcome := adapter.acceptLoad(started.loadRequest.ID, []utils.GoVersion{{
			Version:   "1.30.0",
			Installed: true,
			Path:      "/p/1.30.0",
		}})
		if outcome.kind != catalogProjectionOutcomeReconciled {
			t.Fatalf("verification outcome kind = %v, want Reconciled", outcome.kind)
		}
		if !outcome.receipt.reconciled || outcome.receipt.operation.kind != catalogMutationInstall {
			t.Fatalf("receipt = %+v, want reconciled install receipt", outcome.receipt)
		}
		gotWarnings := outcome.receipt.operation.installWarnings
		if len(gotWarnings) != 1 || gotWarnings[0].Kind != install.WarningCleanup {
			t.Fatalf("receipt warnings = %#v, want copied cleanup warning", gotWarnings)
		}
	})

	t.Run("lifecycle warnings and shim state are copied", func(t *testing.T) {
		adapter := newCatalogProjectionAdapter(testTheme())
		operation := adapter.startMutation(catalogMutationActivation, "1.30.0")
		warning := &lifecycle.CleanupWarning{
			Operation: state.OperationActivate,
			Path:      "/old/shim",
			Err:       errors.New("cleanup failed"),
		}
		warnings := []lifecycle.Warning{warning}
		started := adapter.completeActivation(operation.id, operation.version, warnings, true)
		if started.kind != catalogProjectionOutcomeLoadStarted {
			t.Fatalf("completion kind = %v, want LoadStarted", started.kind)
		}
		warnings[0] = &lifecycle.CleanupWarning{
			Operation: state.OperationActivate,
			Path:      "/replacement",
			Err:       errors.New("replacement warning"),
		}

		other := adapter.startLoad(catalogLoadPurposeRefresh)
		if outcome := adapter.failLoad(other.loadRequest.ID, errors.New("not verification")); outcome.kind != catalogProjectionOutcomeStale {
			t.Fatalf("non-verification failure outcome = %v, want Stale", outcome.kind)
		}
		outcome := adapter.acceptLoad(started.loadRequest.ID, []utils.GoVersion{{
			Version:   "1.30.0",
			Installed: true,
			Active:    true,
			Path:      "/p/1.30.0",
		}})
		if outcome.kind != catalogProjectionOutcomeReconciled {
			t.Fatalf("verification outcome kind = %v, want Reconciled", outcome.kind)
		}
		got := outcome.receipt.operation
		if !outcome.receipt.reconciled || got.kind != catalogMutationActivation || !got.shimInPath {
			t.Fatalf("receipt = %+v, want reconciled activation with shim in PATH", outcome.receipt)
		}
		if len(got.lifecycleWarnings) != 1 || got.lifecycleWarnings[0] != warning {
			t.Fatalf("receipt warnings = %#v, want original copied warning", got.lifecycleWarnings)
		}
	})
}

func TestCatalogProjectionAdapterCommittedWarningPreservesProjection(t *testing.T) {
	tests := []struct {
		name     string
		versions []utils.GoVersion
		start    func(*catalogProjectionAdapter) catalogOperation
		complete func(*catalogProjectionAdapter, catalogOperation) catalogProjectionOutcome
		version  string
	}{
		{
			name:     "invalid install path",
			versions: []utils.GoVersion{{Version: "1.30.0"}},
			start: func(adapter *catalogProjectionAdapter) catalogOperation {
				return adapter.startMutation(catalogMutationInstall, "1.30.0")
			},
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeInstall(operation.id, operation.version, "", nil)
			},
			version: "1.30.0",
		},
		{
			name:     "activation of uninstalled version",
			versions: []utils.GoVersion{{Version: "1.29.0"}},
			start: func(adapter *catalogProjectionAdapter) catalogOperation {
				return adapter.startMutation(catalogMutationActivation, "1.29.0")
			},
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeActivation(operation.id, operation.version, nil, true)
			},
			version: "1.29.0",
		},
		{
			name: "deletion of active version",
			versions: []utils.GoVersion{{
				Version:   "1.28.0",
				Installed: true,
				Active:    true,
				Path:      "/p/1.28.0",
			}},
			start: func(adapter *catalogProjectionAdapter) catalogOperation {
				return adapter.startMutation(catalogMutationDeletion, "1.28.0")
			},
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeDeletion(operation.id, operation.version, nil)
			},
			version: "1.28.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapterTestFixture(t, tt.versions)
			before, ok := adapter.lookup(tt.version)
			if !ok {
				t.Fatalf("lookup(%q) before completion not found", tt.version)
			}
			operation := tt.start(&adapter)

			outcome := tt.complete(&adapter, operation)
			if outcome.kind != catalogProjectionOutcomeCommittedWarning || outcome.err == nil {
				t.Fatalf("completion outcome = %+v, want CommittedWarning with error", outcome)
			}
			if outcome.loadRequest != (catalogLoadRequest{}) {
				t.Fatalf("load request = %+v, want zero request", outcome.loadRequest)
			}
			if phase := adapter.operationPhase(); phase != catalogOperationPhaseIdle {
				t.Fatalf("phase = %v, want Idle", phase)
			}
			if activity := adapter.activityState(); activity != (catalogActivity{kind: catalogActivityIdle}) {
				t.Fatalf("activity = %+v, want Idle", activity)
			}
			after, ok := adapter.lookup(tt.version)
			if !ok || !reflect.DeepEqual(after, before) {
				t.Fatalf("lookup after warning = %+v, found=%v, want prior %+v", after, ok, before)
			}
		})
	}
}

func TestCatalogProjectionAdapterNotFoundStartsReconciliation(t *testing.T) {
	tests := []struct {
		name     string
		kind     catalogMutationKind
		complete func(*catalogProjectionAdapter, catalogOperation) catalogProjectionOutcome
	}{
		{
			name: "install",
			kind: catalogMutationInstall,
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeInstall(operation.id, operation.version, "/p/missing", nil)
			},
		},
		{
			name: "activation",
			kind: catalogMutationActivation,
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeActivation(operation.id, operation.version, nil, true)
			},
		},
		{
			name: "deletion",
			kind: catalogMutationDeletion,
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeDeletion(operation.id, operation.version, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapter(testTheme())
			operation := adapter.startMutation(tt.kind, "1.30.0")
			outcome := tt.complete(&adapter, operation)

			if outcome.kind != catalogProjectionOutcomeLoadStarted || outcome.loadRequest.ID == 0 {
				t.Fatalf("completion outcome = %+v, want reconciliation load", outcome)
			}
			if !errors.Is(outcome.err, errCatalogNotFound) {
				t.Fatalf("completion error = %v, want errCatalogNotFound", outcome.err)
			}
			if phase := adapter.operationPhase(); phase != catalogOperationPhaseReconciling {
				t.Fatalf("phase = %v, want Reconciling", phase)
			}
			if activity := adapter.activityState(); activity != (catalogActivity{
				kind:    catalogActivityReconciling,
				version: "1.30.0",
			}) {
				t.Fatalf("activity = %+v, want reconciling 1.30.0", activity)
			}
		})
	}
}

func TestCatalogProjectionAdapterRefreshStartedDuringMutationKeepsMutationActivity(t *testing.T) {
	adapter := newCatalogProjectionAdapterTestFixture(t, []utils.GoVersion{{Version: "1.30.0"}})
	adapter.startMutation(catalogMutationInstall, "1.30.0")

	load := adapter.startLoad(catalogLoadPurposeRefresh)
	if load.kind != catalogProjectionOutcomeLoadStarted {
		t.Fatalf("refresh outcome kind = %v, want LoadStarted", load.kind)
	}
	if phase := adapter.operationPhase(); phase != catalogOperationPhaseMutating {
		t.Fatalf("phase = %v, want Mutating", phase)
	}
	if activity := adapter.activityState(); activity != (catalogActivity{
		kind:    catalogActivityInstalling,
		version: "1.30.0",
	}) {
		t.Fatalf("activity = %+v, want installing 1.30.0", activity)
	}
}

func TestCatalogProjectionAdapterReconciliationRequestIsIdentifiedByPurpose(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())

	refresh := adapter.startLoad(catalogLoadPurposeRefresh)
	if refresh.loadRequest.Purpose != catalogLoadPurposeRefresh {
		t.Fatalf("refresh purpose = %q, want refresh", refresh.loadRequest.Purpose)
	}
	if adapter.isReconciliationRequest(refresh.loadRequest.ID) {
		t.Fatal("refresh request reported as reconciliation request")
	}
	if outcome := adapter.acceptLoad(refresh.loadRequest.ID, nil); outcome.kind == catalogProjectionOutcomeReconciled {
		t.Fatal("refresh response reported reconciliation success")
	}

	operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
	started := adapter.completeInstall(operation.id, operation.version, "/p/1.30.0", nil)
	if started.kind != catalogProjectionOutcomeLoadStarted {
		t.Fatalf("completion kind = %v, want LoadStarted", started.kind)
	}
	if started.loadRequest.Purpose != catalogLoadPurposeReconcile {
		t.Fatalf("reconciliation purpose = %q, want reconcile", started.loadRequest.Purpose)
	}
	if !adapter.isReconciliationRequest(started.loadRequest.ID) {
		t.Fatal("reconciliation request not identified by purpose")
	}
	if adapter.isReconciliationRequest(started.loadRequest.ID + 1) {
		t.Fatal("unknown request id reported as reconciliation request")
	}
}

func TestCatalogProjectionAdapterLoadDuringReconciliationIsRejected(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())
	operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
	started := adapter.completeInstall(operation.id, operation.version, "/p/1.30.0", nil)
	if started.kind != catalogProjectionOutcomeLoadStarted {
		t.Fatalf("completion kind = %v, want LoadStarted", started.kind)
	}

	if outcome := adapter.startLoad(catalogLoadPurposeRefresh); outcome.kind != catalogProjectionOutcomeStale {
		t.Fatalf("refresh during reconciliation = %v, want Stale", outcome.kind)
	}
	if phase := adapter.operationPhase(); phase != catalogOperationPhaseReconciling {
		t.Fatalf("phase = %v, want Reconciling", phase)
	}

	outcome := adapter.acceptLoad(started.loadRequest.ID, []utils.GoVersion{{
		Version:   "1.30.0",
		Installed: true,
		Path:      "/p/1.30.0",
	}})
	if outcome.kind != catalogProjectionOutcomeReconciled {
		t.Fatalf("reconciliation outcome kind = %v, want Reconciled", outcome.kind)
	}
	if phase := adapter.operationPhase(); phase != catalogOperationPhaseIdle {
		t.Fatalf("phase after reconciliation = %v, want Idle", phase)
	}
	if id := adapter.activeOperationID(); id != 0 {
		t.Fatalf("active operation id after reconciliation = %d, want 0", id)
	}
}

func TestCatalogProjectionAdapterStaleCompletionCannotOverwriteReconciliationReceipt(t *testing.T) {
	tests := []struct {
		name     string
		complete func(*catalogProjectionAdapter, catalogOperation) catalogProjectionOutcome
	}{
		{
			name: "duplicate operation id with stale completion kind",
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeActivation(
					operation.id,
					operation.version,
					nil,
					true,
				)
			},
		},
		{
			name: "unknown completion",
			complete: func(adapter *catalogProjectionAdapter, operation catalogOperation) catalogProjectionOutcome {
				return adapter.completeInstall(
					operation.id+100,
					operation.version,
					"/late",
					[]install.Warning{{Kind: install.WarningIntegrityUnavailable}},
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapter(testTheme())
			operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
			originalWarnings := []install.Warning{{Kind: install.WarningCleanup, Err: errors.New("original")}}
			started := adapter.completeInstall(
				operation.id,
				operation.version,
				"/original",
				originalWarnings,
			)
			if started.kind != catalogProjectionOutcomeLoadStarted {
				t.Fatalf("first completion kind = %v, want LoadStarted", started.kind)
			}

			late := tt.complete(&adapter, operation)
			if late.kind != catalogProjectionOutcomeStale {
				t.Fatalf("late completion kind = %v, want Stale", late.kind)
			}

			outcome := adapter.acceptLoad(started.loadRequest.ID, []utils.GoVersion{{
				Version:   "1.30.0",
				Installed: true,
				Path:      "/original",
			}})
			if outcome.kind != catalogProjectionOutcomeReconciled {
				t.Fatalf("verification outcome kind = %v, want Reconciled", outcome.kind)
			}
			receipt := outcome.receipt.operation
			if len(receipt.installWarnings) != 1 ||
				receipt.installWarnings[0].Kind != install.WarningCleanup {
				t.Fatalf("receipt warnings = %#v, want original cleanup warning", receipt.installWarnings)
			}
		})
	}
}

func TestCatalogProjectionAdapterRejectsSecondMutationWithoutChangingActivity(t *testing.T) {
	tests := []struct {
		name         string
		arrange      func(*catalogProjectionAdapter)
		wantPhase    catalogOperationPhase
		wantActivity catalogActivity
	}{
		{
			name: "while mutating",
			arrange: func(adapter *catalogProjectionAdapter) {
				adapter.startMutation(catalogMutationInstall, "1.30.0")
			},
			wantPhase: catalogOperationPhaseMutating,
			wantActivity: catalogActivity{
				kind:    catalogActivityInstalling,
				version: "1.30.0",
			},
		},
		{
			name: "while reconciling",
			arrange: func(adapter *catalogProjectionAdapter) {
				operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
				adapter.completeInstall(operation.id, operation.version, "/p/1.30.0", nil)
			},
			wantPhase: catalogOperationPhaseReconciling,
			wantActivity: catalogActivity{
				kind:    catalogActivityReconciling,
				version: "1.30.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapter(testTheme())
			tt.arrange(&adapter)

			if operation := adapter.startMutation(catalogMutationDeletion, "1.29.0"); operation.id != 0 {
				t.Fatalf("second operation = %+v, want zero operation", operation)
			}
			if phase := adapter.operationPhase(); phase != tt.wantPhase {
				t.Fatalf("phase = %v, want %v", phase, tt.wantPhase)
			}
			if activity := adapter.activityState(); activity != tt.wantActivity {
				t.Fatalf("activity = %+v, want %+v", activity, tt.wantActivity)
			}
		})
	}
}

func TestCatalogProjectionAdapterRejectsProgressOfOtherOperation(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())
	operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
	accepted := install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 10,
		BytesTotal:    100,
	}
	if !adapter.applyProgress(operation.id, accepted) {
		t.Fatal("applyProgress() = false for the active operation, want true")
	}

	if adapter.applyProgress(operation.id+1, install.Progress{
		Version: "1.30.0",
		Stage:   install.StageVerify,
	}) {
		t.Fatal("applyProgress() = true for a foreign operation, want false")
	}

	got, ok := adapter.activityState().installProgress()
	if !ok || got != accepted {
		t.Fatalf("progress = %+v (ok = %v), want the accepted measurement unchanged", got, ok)
	}
}

func TestCatalogProjectionAdapterProgressLifetime(t *testing.T) {
	measurement := install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 10,
		BytesTotal:    100,
	}
	tests := []struct {
		name         string
		act          func(*catalogProjectionAdapter)
		wantProgress install.Progress
		wantOK       bool
		wantKind     catalogActivityKind
	}{
		{
			name:     "cleared when the operation ends",
			act:      func(adapter *catalogProjectionAdapter) { adapter.toIdle() },
			wantKind: catalogActivityIdle,
		},
		{
			name:     "cleared when the operation is reconciled",
			act:      func(adapter *catalogProjectionAdapter) { adapter.beginReconcile() },
			wantKind: catalogActivityReconciling,
		},
		{
			name: "kept while the catalog refreshes",
			act: func(adapter *catalogProjectionAdapter) {
				adapter.startLoad(catalogLoadPurposeRefresh)
			},
			wantProgress: measurement,
			wantOK:       true,
			wantKind:     catalogActivityInstalling,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newCatalogProjectionAdapter(testTheme())
			operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
			if !adapter.applyProgress(operation.id, measurement) {
				t.Fatal("applyProgress() = false, want the measurement recorded")
			}

			tt.act(&adapter)

			activity := adapter.activityState()
			if activity.kind != tt.wantKind {
				t.Fatalf("activity kind = %v, want %v", activity.kind, tt.wantKind)
			}
			got, ok := activity.installProgress()
			if ok != tt.wantOK || got != tt.wantProgress {
				t.Fatalf("progress = %+v (ok = %v), want %+v (ok = %v)", got, ok, tt.wantProgress, tt.wantOK)
			}
		})
	}
}

func TestCatalogProjectionAdapterReportsNoProgressBeforeFirstMeasurement(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())
	adapter.startMutation(catalogMutationInstall, "1.30.0")

	activity := adapter.activityState()
	if activity.kind != catalogActivityInstalling {
		t.Fatalf("activity kind = %v, want Installing", activity.kind)
	}
	if got, ok := activity.installProgress(); ok {
		t.Fatalf("installProgress() = %+v, ok = true, want ok = false before the first measurement", got)
	}
}

// beginLoad is reachable with a live measurement only through the phase it
// sets, so the retained progress is asserted on the state rather than through
// activityState. Clearing it here would blank the progress bar on a refresh
// pressed during an install.
func TestCatalogProjectionAdapterKeepsProgressAcrossBeginLoad(t *testing.T) {
	adapter := newCatalogProjectionAdapter(testTheme())
	operation := adapter.startMutation(catalogMutationInstall, "1.30.0")
	measurement := install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 10,
		BytesTotal:    100,
	}
	if !adapter.applyProgress(operation.id, measurement) {
		t.Fatal("applyProgress() = false, want the measurement recorded")
	}

	adapter.beginLoad()

	if got := adapter.state.progress; got != measurement {
		t.Fatalf("progress = %+v, want %+v kept across beginLoad()", got, measurement)
	}
}
