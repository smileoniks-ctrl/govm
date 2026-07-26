package model

// Tests for the Bubbletea install adapter (install_cmd.go). These tests
// inject a fake install function so they assert request mapping, the
// deadline-bound context, and the success/warning/failure state machine
// without touching disk or network.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// installProgressCapture is a fake progress-reporting install function
// that records the request and context it was called with and returns a
// canned result.
func installProgressCapture(
	req *install.Request,
	ctxp *context.Context,
	result install.Result,
	err error,
) installProgressFunc {
	return func(ctx context.Context, r install.Request, _ install.ProgressReporter) (install.Result, error) {
		*req = r
		*ctxp = ctx
		return result, err
	}
}

// TestInstallVersionProgressCmd_MapsRequestAndTimeout verifies that the
// install command dispatched by the TUI maps every GoVersion field
// (including the integrity metadata) into the install.Request and hands
// the core a deadline-bound context, then resolves to an
// installSuccessMsg carrying the whole result.
func TestInstallVersionProgressCmd_MapsRequestAndTimeout(t *testing.T) {
	m := newTestModel(t)
	var captured install.Request
	var capturedCtx context.Context
	m.installWithProgress = installProgressCapture(&captured, &capturedCtx,
		install.Result{Version: "1.30.0", Path: "/p/1.30.0"}, nil)

	src := utils.GoVersion{
		Version:  "1.30.0",
		Filename: "go1.30.0.darwin-arm64.tar.gz",
		URL:      "https://go.dev/dl/go1.30.0.darwin-arm64.tar.gz",
		SHA256:   "deadbeef",
		Size:     987654,
	}
	const operationID = 42
	msg := m.installVersionProgressCmd(operationID, src)()

	// Every field, including the propagated integrity metadata, is
	// mapped into the request.
	if captured.Version != "1.30.0" ||
		captured.Filename != src.Filename ||
		captured.URL != src.URL ||
		captured.SHA256 != "deadbeef" ||
		captured.Size != 987654 {
		t.Fatalf("captured request = %+v, want mapped GoVersion", captured)
	}

	// The context must carry a deadline (~30 minute budget).
	deadline, ok := capturedCtx.Deadline()
	if !ok {
		t.Fatal("expected a deadline on the install context")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > installTimeout {
		t.Fatalf("deadline remaining = %v, want within %v", remaining, installTimeout)
	}

	// Success message carries the whole install.Result.
	success, ok := msg.(installSuccessMsg)
	if !ok {
		t.Fatalf("expected installSuccessMsg, got %T", msg)
	}
	if success.Version != "1.30.0" || success.Path != "/p/1.30.0" {
		t.Fatalf("success result = %+v", success)
	}
	if success.OperationID != operationID {
		t.Fatalf("operation ID = %d, want %d", success.OperationID, operationID)
	}
}

// TestInstallVersionProgressCmd_FailureReturnsTypedError verifies a
// failing install produces an installFailureMsg carrying the requested
// version and the typed (stage-aware) error.
func TestInstallVersionProgressCmd_FailureReturnsTypedError(t *testing.T) {
	m := newTestModel(t)
	installErr := &install.Error{Stage: install.StageDownload, Err: errors.New("network down")}
	m.installWithProgress = func(
		context.Context,
		install.Request,
		install.ProgressReporter,
	) (install.Result, error) {
		return install.Result{}, installErr
	}

	msg := m.installVersionProgressCmd(42, utils.GoVersion{Version: "1.30.0"})()

	failure, ok := msg.(installFailureMsg)
	if !ok {
		t.Fatalf("expected installFailureMsg, got %T", msg)
	}
	if failure.Version != "1.30.0" {
		t.Fatalf("version = %q, want 1.30.0", failure.Version)
	}
	if !errors.Is(failure.Err, installErr) {
		t.Fatalf("err = %v, want the typed install.Error", failure.Err)
	}
}

// TestInstallVersionProgressCmd_NilInstallerDoesNotPanic verifies a
// model whose installer was never bound surfaces a failure instead of
// panicking, keeping bare test constructors safe.
func TestInstallVersionProgressCmd_NilInstallerDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("installVersionProgressCmd panicked on nil installer: %v", r)
		}
	}()
	m := newTestModel(t)
	m.installGo = nil
	m.installWithProgress = nil

	msg := m.installVersionProgressCmd(42, utils.GoVersion{Version: "1.30.0"})()
	if _, ok := msg.(installFailureMsg); !ok {
		t.Fatalf("expected installFailureMsg for nil installer, got %T", msg)
	}
}

// TestInstallVersionProgressCmd_FallsBackToPlainInstaller verifies the
// progress path still installs through the plain installer when no
// progress-reporting installer is bound.
func TestInstallVersionProgressCmd_FallsBackToPlainInstaller(t *testing.T) {
	m := newTestModel(t)
	m.installWithProgress = nil
	var captured install.Request
	m.installGo = func(_ context.Context, r install.Request) (install.Result, error) {
		captured = r
		return install.Result{Version: r.Version, Path: "/p/" + r.Version}, nil
	}

	msg := m.installVersionProgressCmd(42, utils.GoVersion{Version: "1.30.0"})()

	if captured.Version != "1.30.0" {
		t.Fatalf("captured request = %+v, want the requested version", captured)
	}
	if _, ok := msg.(installSuccessMsg); !ok {
		t.Fatalf("expected installSuccessMsg, got %T", msg)
	}
}

// TestHandleInstallKey_IssuesInstallCmd verifies the 'i' key on the
// Available tab dispatches the private install command and records the
// in-flight version.
func TestHandleInstallKey_IssuesInstallCmd(t *testing.T) {
	m := newVersionCacheTestModel(t)
	m.CurrentTab = AvailableTab
	// The seeded catalog sorts desc: 1.26.0, 1.25.0 (uninstalled), 1.24.4.
	// Select the uninstalled 1.25.0 entry by identity.
	target := -1
	for i := 0; i < len(m.projection.availableModel().Items()); i++ {
		if listItemName(m, i) == "1.25.0" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("could not find uninstalled 1.25.0 entry to install")
	}
	m.projection.selectAvailable(target)

	// Sentinel fake: any install run flips this flag.
	called := false
	m.installGo = func(ctx context.Context, r install.Request) (install.Result, error) {
		called = true
		return install.Result{Version: r.Version, Path: "/p/" + r.Version}, nil
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'i'})
	got := updated.(Model)

	if activity := got.projection.activityState(); activity.kind != catalogActivityInstalling || activity.version != "1.25.0" {
		t.Fatalf("activity = %+v, want installing 1.25.0", activity)
	}
	if cmd == nil {
		t.Fatal("expected an install command to be dispatched")
	}
	// Executing the command drives the fake and yields the success msg.
	msg := cmd()
	if !called {
		t.Fatal("expected the injected install function to be invoked")
	}
	if _, ok := msg.(installSuccessMsg); !ok {
		t.Fatalf("expected installSuccessMsg, got %T", msg)
	}
}

// TestInstallSuccess_PreservesExistingSuccessText verifies that a
// successful install with no warnings produces the exact historical
// success text via the direct completion path.
func TestInstallSuccess_PreservesExistingSuccessText(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.25.0")
	updated, _ := m.Update(installSuccessMsg{
		OperationID: operation.id,
		Version:     "1.25.0",
		Path:        "/new/1.25",
	})
	got := updated.(Model)

	if got.Status.Text() != "Successfully installed Go 1.25.0" {
		t.Fatalf("status = %q, want historical success text", got.Status.Text())
	}
	if got.Status.Kind() != "success" {
		t.Fatalf("kind = %q, want success", got.Status.Kind())
	}
	if activity := got.projection.activityState(); activity.kind != catalogActivityIdle {
		t.Fatalf("activity = %+v, want idle after successful install", activity)
	}
}

func TestInstallSuccessRefreshesDiskUsage(t *testing.T) {
	m := newVersionCacheTestModel(t)
	m.diskUsage = func(context.Context) (prune.Summary, error) {
		return prune.Summary{
			VersionBytes: map[string]int64{"1.25.0": 4096},
		}, nil
	}
	operation := m.projection.startMutation(catalogMutationInstall, "1.25.0")
	updated, cmd := m.Update(installSuccessMsg{
		OperationID: operation.id,
		Version:     "1.25.0",
		Path:        "/new/1.25",
	})
	if cmd == nil {
		t.Fatal("expected disk usage refresh command")
	}
	updatedModel := updated.(Model)
	updated, _ = updatedModel.Update(m.diskUsageCmd()())
	got := updated.(Model)
	for _, row := range got.projection.installedModel().Rows() {
		if row[0] == "1.25.0" {
			if row[2] != "4.0 KiB" {
				t.Fatalf("size column = %q, want 4.0 KiB", row[2])
			}
			return
		}
	}
	t.Fatal("installed version 1.25.0 not found")
}

// TestInstallSuccess_WarningsProduceWarningStatus verifies that install
// warnings surface as a single global warning status while still
// marking the version installed.
func TestInstallSuccess_WarningsProduceWarningStatus(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.25.0")
	updated, _ := m.Update(installSuccessMsg{
		OperationID: operation.id,
		Version:     "1.25.0",
		Path:        "/new/1.25",
		Warnings: []install.Warning{
			{Kind: install.WarningIntegrityUnavailable},
		},
	})
	got := updated.(Model)

	if got.Status.Kind() != "warning" {
		t.Fatalf("kind = %q, want warning", got.Status.Kind())
	}
	if !strings.Contains(got.Status.Text(), "with warnings") {
		t.Fatalf("status = %q, want warnings surfaced", got.Status.Text())
	}
	v, ok := got.projection.lookup("1.25.0")
	if !ok || !v.Installed {
		t.Fatalf("version still marked uninstalled despite warnings: %+v", v)
	}
}

// TestInstallFailure_ClearsStateAndReportsError verifies the failure
// path clears loading/install state, drops the reconcile context, and
// reports the phase-aware typed error.
func TestInstallFailure_ClearsStateAndReportsError(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.30.0")

	updated, _ := m.Update(installFailureMsg{
		OperationID: operation.id,
		Version:     "1.30.0",
		Err: &install.Error{
			Stage: install.StageExtract,
			Err:   errors.New("corrupt archive"),
		},
	})
	got := updated.(Model)

	if activity := got.projection.activityState(); activity.kind != catalogActivityIdle {
		t.Fatalf("activity = %+v, want idle on failure", activity)
	}
	if phase := got.projection.operationPhase(); phase != catalogOperationPhaseIdle {
		t.Fatalf("operation phase = %v, want idle on failure", phase)
	}
	if !strings.Contains(got.Status.Text(), "Failed to install Go 1.30.0") {
		t.Fatalf("status = %q, want failure prefix", got.Status.Text())
	}
	if !strings.Contains(got.Status.Text(), "corrupt archive") {
		t.Fatalf("status = %q, want underlying error surfaced", got.Status.Text())
	}
	if !strings.Contains(got.Status.Text(), "extraction") {
		t.Fatalf("status = %q, want phase-aware (stage) text", got.Status.Text())
	}
}

// TestUnknownCompletionReconciliationPreservesWarnings verifies that
// warnings carried by a completion that cannot be confirmed against the
// catalog survive the reconciliation fetch and surface in the final
// status.
func TestUnknownCompletionReconciliationPreservesWarnings(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.30.0")

	// 1. Completion for a version absent from the catalog triggers a
	//    reconciliation carrying the install warnings.
	updated, cmd := m.Update(installSuccessMsg{
		OperationID: operation.id,
		Version:     "1.30.0",
		Path:        "/p/1.30.0",
		Warnings: []install.Warning{
			{Kind: install.WarningCleanup, Err: errors.New("temp file busy")},
		},
	})
	m = updated.(Model)

	if activity := m.projection.activityState(); activity.kind != catalogActivityReconciling {
		t.Fatalf("activity = %+v, want reconciling unknown completion", activity)
	}

	// 2. The reconciliation fetch returns a catalog that now includes
	//    the completed version, installed on disk.
	fresh := projectionVersions(m, utils.GoVersion{
		Version:   "1.30.0",
		Filename:  "go1.30.0.darwin-arm64.tar.gz",
		Installed: true,
		Path:      "/p/1.30.0",
	})
	updated, _ = m.Update(catalogLoadedMsg{
		RequestID: catalogRequestID(t, cmd),
		Versions:  fresh,
	})
	got := updated.(Model)

	if got.Status.Kind() != "warning" {
		t.Fatalf("status kind = %q, want warning (warnings survived)", got.Status.Kind())
	}
	if !strings.Contains(got.Status.Text(), "with warnings") ||
		!strings.Contains(got.Status.Text(), "temp file busy") {
		t.Fatalf("status = %q, want surviving warnings", got.Status.Text())
	}
}
