package model

import (
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/prune"
)

// TestInstalledSummaryHidesAbsentInterruptedDownloads pins the summary
// to what actually exists on disk. govm removes a download's .part file
// on success, on failure, and again during the next install's orphan
// sweep, so a permanently rendered field reported "0 B" in every state a
// user could observe.
func TestInstalledSummaryHidesAbsentInterruptedDownloads(t *testing.T) {
	line := renderInstalledSummary(prune.Summary{
		InstalledBytes:   863734169,
		DownloadBytes:    0,
		ReclaimableBytes: 635121234,
	})

	if strings.Contains(line, "Interrupted") || strings.Contains(line, "Downloads") {
		t.Errorf("summary = %q, want no download field when nothing is interrupted", line)
	}
	for _, want := range []string{"Installed:", "Reclaimable:"} {
		if !strings.Contains(line, want) {
			t.Errorf("summary = %q, want it to contain %q", line, want)
		}
	}
}

// TestInstalledSummaryReportsInterruptedDownloads verifies the field
// appears — under a name describing what it measures — exactly when a
// crashed install has left debris behind.
func TestInstalledSummaryReportsInterruptedDownloads(t *testing.T) {
	line := renderInstalledSummary(prune.Summary{
		InstalledBytes:   863734169,
		DownloadBytes:    78123456,
		ReclaimableBytes: 635121234,
	})

	if !strings.Contains(line, "Interrupted:") {
		t.Errorf("summary = %q, want an Interrupted field when a .part file survives", line)
	}
	if !strings.Contains(line, prune.FormatBytes(78123456)) {
		t.Errorf("summary = %q, want the interrupted size rendered", line)
	}
}
