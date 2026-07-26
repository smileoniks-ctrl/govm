package install

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestInstallWithProgressReportsStagesAndBytes(t *testing.T) {
	s, _ := newTestService(t, "1.22.0")
	const archive = "archive-content"
	req := makeRequest("1.22.0", func(r *Request) {
		r.Size = int64(len(archive))
	})
	s.doer = &fakeDoer{respond: func(*http.Request) (*http.Response, error) {
		return okResponse([]byte(archive)), nil
	}}

	var events []Progress
	if _, err := s.InstallWithProgress(context.Background(), req, ProgressReporterFunc(func(event Progress) {
		events = append(events, event)
	})); err != nil {
		t.Fatalf("InstallWithProgress() error = %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	if events[0].Stage != StageValidate {
		t.Fatalf("first stage = %s, want %s", events[0].Stage, StageValidate)
	}

	wantStages := []Stage{
		StageValidate,
		StageLock,
		StagePrepare,
		StageDownload,
		StageIntegrity,
		StageExtract,
		StageVerify,
		StageCommit,
		StageCleanup,
	}
	stageIndex := 0
	for _, event := range events {
		if stageIndex < len(wantStages) && event.Stage == wantStages[stageIndex] {
			stageIndex++
		}
	}
	if stageIndex != len(wantStages) {
		t.Fatalf("stages = %v, want ordered stages %v", progressStages(events), wantStages)
	}

	lastDownload := Progress{}
	for _, event := range events {
		if event.Stage == StageDownload && event.BytesReceived > lastDownload.BytesReceived {
			lastDownload = event
		}
	}
	if lastDownload.BytesReceived != int64(len(archive)) ||
		lastDownload.BytesTotal != int64(len(archive)) {
		t.Fatalf("last download = %+v, want %d/%d bytes", lastDownload, len(archive), len(archive))
	}
}

func TestInstallWithProgressReportsReceivedBytesWithoutTotal(t *testing.T) {
	s, _ := newTestService(t, "1.22.0")
	const archive = "archive-content"
	var events []Progress

	if _, err := s.InstallWithProgress(
		context.Background(),
		makeRequest("1.22.0"),
		ProgressReporterFunc(func(event Progress) {
			events = append(events, event)
		}),
	); err != nil {
		t.Fatalf("InstallWithProgress() error = %v", err)
	}

	var lastDownload Progress
	for _, event := range events {
		if event.Stage == StageDownload && event.BytesReceived > lastDownload.BytesReceived {
			lastDownload = event
		}
	}
	if lastDownload.BytesReceived != int64(len(archive)) || lastDownload.BytesTotal != 0 {
		t.Fatalf("last download = %+v, want %d bytes and unknown total", lastDownload, len(archive))
	}
}

func progressStages(events []Progress) string {
	stages := make([]string, len(events))
	for i, event := range events {
		stages[i] = event.Stage.String()
	}
	return strings.Join(stages, ", ")
}
