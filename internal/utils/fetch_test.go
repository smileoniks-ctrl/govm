package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// goDevPayload mirrors the go.dev /dl/?mode=json shape, including the
// sha256 and size fields that goDevFile now decodes.
type goDevPayload []struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
		Size     int    `json:"size"`
	} `json:"files"`
}

func TestFetchGoDevReleases_Success(t *testing.T) {
	payload := goDevPayload{
		{
			Version: "go1.22.0",
			Stable:  true,
			Files: []struct {
				Filename string `json:"filename"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Kind     string `json:"kind"`
				SHA256   string `json:"sha256"`
				Size     int    `json:"size"`
			}{
				{Filename: "go1.22.0.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64", Kind: "archive", SHA256: "abc123", Size: 12345},
			},
		},
		{
			Version: "go1.21.5",
			Stable:  true,
			Files: []struct {
				Filename string `json:"filename"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Kind     string `json:"kind"`
				SHA256   string `json:"sha256"`
				Size     int    `json:"size"`
			}{
				{Filename: "go1.21.5.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", Kind: "archive", SHA256: "def456", Size: 67890},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	releases, err := fetchGoDevReleases(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	// Normalisation: "go" prefix stripped at the boundary (decision B1).
	if releases[0].Version != "1.22.0" {
		t.Errorf("Version[0] = %q, want %q", releases[0].Version, "1.22.0")
	}
	if releases[1].Version != "1.21.5" {
		t.Errorf("Version[1] = %q, want %q", releases[1].Version, "1.21.5")
	}
	if !releases[0].Stable {
		t.Errorf("Stable[0] = false, want true")
	}
	if len(releases[0].Files) != 1 {
		t.Fatalf("Files[0] len = %d, want 1", len(releases[0].Files))
	}
	if got := releases[0].Files[0].Filename; got != "go1.22.0.darwin-arm64.tar.gz" {
		t.Errorf("Filename = %q, want %q", got, "go1.22.0.darwin-arm64.tar.gz")
	}
	if got := releases[0].Files[0].Kind; got != "archive" {
		t.Errorf("Kind = %q, want archive", got)
	}
	// Integrity metadata must be decoded so it can flow into the
	// install core's verification step.
	if got := releases[0].Files[0].SHA256; got != "abc123" {
		t.Errorf("SHA256 = %q, want %q", got, "abc123")
	}
	if got := releases[0].Files[0].Size; got != 12345 {
		t.Errorf("Size = %d, want %d", got, 12345)
	}
}

func TestFetchGoDevReleases_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	releases, err := fetchGoDevReleases(server.Client(), server.URL)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil with %d releases", len(releases))
	}
	if !strings.Contains(err.Error(), "fetch go.dev releases") {
		t.Errorf("error should carry the function prefix, got: %v", err)
	}
}

func TestFetchGoDevReleases_ConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // force a connection error on the next call

	releases, err := fetchGoDevReleases(server.Client(), server.URL)
	if err == nil {
		t.Fatalf("expected error for closed server, got %d releases", len(releases))
	}
	if !strings.Contains(err.Error(), "fetch go.dev releases") {
		t.Errorf("error should carry the function prefix, got: %v", err)
	}
}

func TestFetchGoDevReleases_BadURL(t *testing.T) {
	// An unparseable URL surfaces the prefix via http.NewRequest.
	_, err := fetchGoDevReleases(http.DefaultClient, "http://[::1]:namedhost")
	if err == nil {
		t.Fatalf("expected error for bad URL")
	}
	if !strings.Contains(err.Error(), "fetch go.dev releases") {
		t.Errorf("error should carry the function prefix, got: %v", err)
	}
}
