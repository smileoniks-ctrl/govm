package godev

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDoer struct {
	response *http.Response
	err      error
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func TestFetchReleases_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"version": "go1.22.0",
				"stable": true,
				"files": [
					{
						"filename": "go1.22.0.linux-amd64.tar.gz",
						"os": "linux",
						"arch": "amd64",
						"kind": "archive",
						"sha256": "abc123",
						"size": 102400
					},
					{
						"filename": "go1.22.0.darwin-arm64.tar.gz",
						"os": "darwin",
						"arch": "arm64",
						"kind": "archive",
						"sha256": "def456",
						"size": 204800
					}
				]
			},
			{
				"version": "go1.21.5",
				"stable": true,
				"files": [
					{
						"filename": "go1.21.5.linux-amd64.tar.gz",
						"os": "linux",
						"arch": "amd64",
						"kind": "archive",
						"sha256": "ghi789",
						"size": 98304
					}
				]
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx := context.Background()

	releases, err := client.FetchReleases(ctx)
	if err != nil {
		t.Fatalf("FetchReleases failed: %v", err)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	if releases[0].Version != "1.22.0" {
		t.Errorf("expected version 1.22.0, got %s", releases[0].Version)
	}
	if !releases[0].Stable {
		t.Errorf("expected release to be stable")
	}
	if len(releases[0].Files) != 2 {
		t.Errorf("expected 2 files in first release, got %d", len(releases[0].Files))
	}

	file := releases[0].Files[0]
	if file.Filename != "go1.22.0.linux-amd64.tar.gz" {
		t.Errorf("expected filename go1.22.0.linux-amd64.tar.gz, got %s", file.Filename)
	}
	if file.OS != "linux" {
		t.Errorf("expected OS linux, got %s", file.OS)
	}
	if file.Arch != "amd64" {
		t.Errorf("expected arch amd64, got %s", file.Arch)
	}
	if file.SHA256 != "abc123" {
		t.Errorf("expected SHA256 abc123, got %s", file.SHA256)
	}
	if file.Size != 102400 {
		t.Errorf("expected size 102400, got %d", file.Size)
	}

	if releases[1].Version != "1.21.5" {
		t.Errorf("expected version 1.21.5, got %s", releases[1].Version)
	}
}

func TestFetchReleases_VersionNormalization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"version": "go1.22.0",
				"stable": true,
				"files": [
					{
						"filename": "go1.22.0.linux-amd64.tar.gz",
						"os": "linux",
						"arch": "amd64",
						"kind": "archive",
						"sha256": "abc",
						"size": 1024
					}
				]
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx := context.Background()

	releases, err := client.FetchReleases(ctx)
	if err != nil {
		t.Fatalf("FetchReleases failed: %v", err)
	}

	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}

	if releases[0].Version != "1.22.0" {
		t.Errorf("expected version normalized to 1.22.0 (no 'go' prefix), got %s", releases[0].Version)
	}
}

func TestFetchReleases_NetworkError(t *testing.T) {
	client := NewClient(&fakeDoer{
		err: errors.New("network timeout"),
	}, "http://unreachable.example.com")

	ctx := context.Background()
	_, err := client.FetchReleases(ctx)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if err.Error() != "fetch go.dev releases: network timeout" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetchReleases_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.FetchReleases(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestFetchReleases_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx := context.Background()

	_, err := client.FetchReleases(ctx)
	if err == nil {
		t.Fatal("expected JSON unmarshal error, got nil")
	}
}

func TestFetchReleases_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx := context.Background()

	releases, err := client.FetchReleases(ctx)
	if err != nil {
		t.Fatalf("FetchReleases failed: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("expected empty releases, got %d", len(releases))
	}
}

func TestFetchReleases_FileMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"version": "go1.22.0",
				"stable": false,
				"files": [
					{
						"filename": "go1.22.0.src.tar.gz",
						"os": "linux",
						"arch": "amd64",
						"kind": "source",
						"sha256": "src123",
						"size": 50000
					},
					{
						"filename": "go1.22.0.linux-amd64.tar.gz",
						"os": "linux",
						"arch": "amd64",
						"kind": "archive",
						"sha256": "archive456",
						"size": 100000
					}
				]
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(http.DefaultClient, server.URL)
	ctx := context.Background()

	releases, err := client.FetchReleases(ctx)
	if err != nil {
		t.Fatalf("FetchReleases failed: %v", err)
	}

	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}

	if releases[0].Stable {
		t.Errorf("expected release to be unstable")
	}

	if len(releases[0].Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(releases[0].Files))
	}

	sourceFile := releases[0].Files[0]
	if sourceFile.Kind != "source" {
		t.Errorf("expected kind source, got %s", sourceFile.Kind)
	}
	if sourceFile.SHA256 != "src123" {
		t.Errorf("expected SHA256 src123, got %s", sourceFile.SHA256)
	}
	if sourceFile.Size != 50000 {
		t.Errorf("expected size 50000, got %d", sourceFile.Size)
	}

	archiveFile := releases[0].Files[1]
	if archiveFile.Kind != "archive" {
		t.Errorf("expected kind archive, got %s", archiveFile.Kind)
	}
	if archiveFile.SHA256 != "archive456" {
		t.Errorf("expected SHA256 archive456, got %s", archiveFile.SHA256)
	}
	if archiveFile.Size != 100000 {
		t.Errorf("expected size 100000, got %d", archiveFile.Size)
	}
}
