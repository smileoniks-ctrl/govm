package utils

import (
	"testing"
)

func releasesForCatalog() []goDevRelease {
	return []goDevRelease{
		{
			Version: "1.22.0",
			Stable:  true,
			Files: []goDevFile{
				{Filename: "go1.22.0.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64"},
				{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"},
			},
		},
		{
			Version: "1.21.5",
			Stable:  true,
			Files: []goDevFile{
				{Filename: "go1.21.5.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64"},
			},
		},
		{
			Version: "1.21.0",
			Stable:  false,
			Files: []goDevFile{
				// darwin/arm64 build exists for an unstable release so we
				// can assert the Stable flag is propagated end-to-end.
				{Filename: "go1.21.0.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64"},
			},
		},
	}
}

func TestBuildVersionCatalog_SelectsMatchingOSArch(t *testing.T) {
	got := buildVersionCatalog(releasesForCatalog(), "darwin", "arm64", nil, "")

	if len(got) != 3 {
		t.Fatalf("expected 3 entries (darwin/arm64 only), got %d: %+v", len(got), got)
	}
	wantVersions := []string{"1.22.0", "1.21.5", "1.21.0"}
	wantStable := []bool{true, true, false}
	for i, w := range wantVersions {
		if got[i].Version != w {
			t.Errorf("got[%d].Version = %q, want %q", i, got[i].Version, w)
		}
		if got[i].Stable != wantStable[i] {
			t.Errorf("got[%d].Stable = %v, want %v", i, got[i].Stable, wantStable[i])
		}
	}
	// The darwin/arm64 file was chosen for the cross-platform release.
	if got[0].Filename != "go1.22.0.darwin-arm64.tar.gz" {
		t.Errorf("got[0].Filename = %q, want darwin/arm64 file", got[0].Filename)
	}
	if got[0].URL != "https://go.dev/dl/go1.22.0.darwin-arm64.tar.gz" {
		t.Errorf("got[0].URL = %q", got[0].URL)
	}
}

func TestBuildVersionCatalog_SkipsReleasesWithoutMatchingOSArch(t *testing.T) {
	// Linux-only release must be skipped when currentOS=darwin.
	releases := []goDevRelease{
		{
			Version: "1.22.0",
			Stable:  true,
			Files: []goDevFile{
				{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"},
			},
		},
	}
	got := buildVersionCatalog(releases, "darwin", "arm64", nil, "")
	if len(got) != 0 {
		t.Errorf("linux-only release should be skipped on darwin, got %v", got)
	}
}

func TestBuildVersionCatalog_MarksInstalledAndActive(t *testing.T) {
	installed := map[string]string{
		"1.22.0": "/home/u/.govm/versions/go1.22.0",
	}
	got := buildVersionCatalog(releasesForCatalog(), "darwin", "arm64", installed, "1.21.5")

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	// 1.22.0 is installed.
	if !got[0].Installed {
		t.Errorf("1.22.0 should be Installed")
	}
	if got[0].Path != "/home/u/.govm/versions/go1.22.0" {
		t.Errorf("1.22.0 Path = %q", got[0].Path)
	}
	if got[0].Active {
		t.Errorf("1.22.0 should not be Active (active=1.21.5)")
	}
	// 1.21.5 is active but not installed.
	if got[1].Installed {
		t.Errorf("1.21.5 should not be Installed")
	}
	if got[1].Path != "" {
		t.Errorf("1.21.5 Path should be empty, got %q", got[1].Path)
	}
	if !got[1].Active {
		t.Errorf("1.21.5 should be Active")
	}
	// 1.21.0 is neither installed nor active.
	if got[2].Installed {
		t.Errorf("1.21.0 should not be Installed")
	}
	if got[2].Active {
		t.Errorf("1.21.0 should not be Active")
	}
}

func TestBuildVersionCatalog_EmptyInputsYieldEmptyResult(t *testing.T) {
	got := buildVersionCatalog(nil, "darwin", "arm64", nil, "")
	if got != nil {
		t.Errorf("nil releases should yield nil, got %v", got)
	}

	got = buildVersionCatalog([]goDevRelease{}, "darwin", "arm64", nil, "")
	if len(got) != 0 {
		t.Errorf("empty releases should yield empty slice, got %v", got)
	}
}

func TestBuildVersionCatalog_FirstMatchingFileWins(t *testing.T) {
	releases := []goDevRelease{
		{
			Version: "1.22.0",
			Stable:  true,
			Files: []goDevFile{
				{Filename: "go1.22.0.darwin-arm64.first.tar.gz", OS: "darwin", Arch: "arm64"},
				{Filename: "go1.22.0.darwin-arm64.second.tar.gz", OS: "darwin", Arch: "arm64"},
			},
		},
	}
	got := buildVersionCatalog(releases, "darwin", "arm64", nil, "")
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Filename != "go1.22.0.darwin-arm64.first.tar.gz" {
		t.Errorf("first matching file should win, got %q", got[0].Filename)
	}
}

func TestBuildVersionCatalog_NoOSArchMatchYieldsNothing(t *testing.T) {
	got := buildVersionCatalog(releasesForCatalog(), "windows", "arm64", nil, "")
	if len(got) != 0 {
		t.Errorf("windows/arm64 has no matching files, expected empty, got %v", got)
	}
}
