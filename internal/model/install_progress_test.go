package model

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/install"
)

func TestInstallProgressMailboxKeepsLatestEvent(t *testing.T) {
	mailbox := newInstallProgressMailbox()
	mailbox.Report(install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 1,
		BytesTotal:    10,
	})
	mailbox.Report(install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 9,
		BytesTotal:    10,
	})

	<-mailbox.notify
	got := mailbox.snapshot()
	if got.BytesReceived != 9 || got.BytesTotal != 10 {
		t.Fatalf("latest progress = %+v, want 9/10 bytes", got)
	}
}

func TestInstallProgressSessionReturnsProgressThenOutcome(t *testing.T) {
	release := make(chan struct{})
	session := &installProgressSession{
		operationID: 42,
		request:     install.Request{Version: "1.30.0"},
		outcomes:    make(chan tea.Msg, 1),
		mailbox:     newInstallProgressMailbox(),
		install: func(
			_ context.Context,
			_ install.Request,
			reporter install.ProgressReporter,
		) (install.Result, error) {
			reporter.Report(install.Progress{
				Version: "1.30.0",
				Stage:   install.StagePrepare,
			})
			<-release
			return install.Result{Version: "1.30.0", Path: "/p/1.30.0"}, nil
		},
	}

	session.start()
	first := session.wait(true)()
	progressMsg, ok := first.(installProgressMsg)
	if !ok {
		t.Fatalf("first message = %T, want installProgressMsg", first)
	}
	if progressMsg.progress.Stage != install.StagePrepare {
		t.Fatalf("progress stage = %s, want %s", progressMsg.progress.Stage, install.StagePrepare)
	}

	close(release)
	second := session.wait(false)()
	if _, ok := second.(installSuccessMsg); !ok {
		t.Fatalf("second message = %T, want installSuccessMsg", second)
	}
}

func TestRenderDownloadStatusUsesBytesAndPercentage(t *testing.T) {
	m := newTestModel(t)
	status := stripANSI(m.renderDownloadStatus(install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 512 * 1024,
		BytesTotal:    1024 * 1024,
	}, 100))

	for _, want := range []string{"Downloading Go 1.30.0", "50%", "512.0 KiB", "1.0 MiB"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status = %q, want %q", status, want)
		}
	}
}

func TestRenderDownloadStatusUnknownTotalUsesSpinnerAndBytes(t *testing.T) {
	m := newTestModel(t)
	status := stripANSI(m.renderDownloadStatus(install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 12 * 1024,
	}, 100))

	if !strings.Contains(status, "Downloading Go 1.30.0") ||
		!strings.Contains(status, "12.0 KiB") {
		t.Fatalf("status = %q, want spinner download and bytes", status)
	}
	if strings.Contains(status, "%") {
		t.Fatalf("status = %q, did not expect percentage with unknown total", status)
	}
}

func TestRenderDownloadStatusFitsNarrowWidth(t *testing.T) {
	m := newTestModel(t)
	const width = 64
	status := m.renderDownloadStatus(install.Progress{
		Version:       "1.30.0",
		Stage:         install.StageDownload,
		BytesReceived: 512 * 1024,
		BytesTotal:    1024 * 1024,
	}, width)

	if got := ansi.StringWidth(status); got > width-2 {
		t.Fatalf("status width = %d, want <= %d: %q", got, width-2, status)
	}
}

func TestInstallProgressIgnoresStaleOperation(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.25.0")
	if !m.projection.applyProgress(operation.id, install.Progress{
		Version: "1.25.0",
		Stage:   install.StageDownload,
	}) {
		t.Fatal("applyProgress() = false, want the measurement recorded")
	}

	cmd := m.handleInstallProgress(installProgressMsg{
		operationID: operation.id + 1,
		progress: install.Progress{
			Version: "1.25.0",
			Stage:   install.StageVerify,
		},
		session: &installProgressSession{},
	})
	if cmd != nil {
		t.Fatal("stale progress should not schedule another poll")
	}
	progress, _ := m.projection.activityState().installProgress()
	if got := progress.Stage; got != install.StageDownload {
		t.Fatalf("stage = %s, want stale event to be ignored", got)
	}
}

func TestComposeStatusUsesInstallProgressStage(t *testing.T) {
	m := newVersionCacheTestModel(t)
	operation := m.projection.startMutation(catalogMutationInstall, "1.25.0")
	if !m.projection.applyProgress(operation.id, install.Progress{
		Version:       "1.25.0",
		Stage:         install.StageIntegrity,
		BytesReceived: 10,
	}) {
		t.Fatal("applyProgress() = false, want the measurement recorded")
	}

	status, kind := m.composeStatus()
	if kind != "info" {
		t.Fatalf("status kind = %q, want info", kind)
	}
	plain := stripANSI(status)
	if !strings.Contains(plain, "Checking integrity Go 1.25.0") {
		t.Fatalf("status = %q, want integrity stage", plain)
	}
}
