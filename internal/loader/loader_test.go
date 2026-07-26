package loader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/adapter/local"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

// Fake adapters for testing orchestration logic in isolation.

type fakeReleaseSource struct {
	releases []Release
	err      error
}

func (f *fakeReleaseSource) FetchReleases(ctx context.Context) ([]Release, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

type fakeLocalVersions struct {
	installed map[string]string
	err       error
}

func (f *fakeLocalVersions) ScanInstalled(ctx context.Context) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.installed, nil
}

type fakeActiveVersion struct {
	active       string
	readErr      error
	pathFallback string
}

func (f *fakeActiveVersion) ReadActive(ctx context.Context) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.active, nil
}

func (f *fakeActiveVersion) GetFromPath(ctx context.Context) string {
	return f.pathFallback
}

func TestLoadVersionCatalog_Success(t *testing.T) {
	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			releases: []Release{
				{
					Version: "1.22.0",
					Stable:  true,
					Files: []FileEntry{
						{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", SHA256: "abc123", Size: 1024},
					},
				},
				{
					Version: "1.21.5",
					Stable:  true,
					Files: []FileEntry{
						{Filename: "go1.21.5.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", SHA256: "def456", Size: 2048},
					},
				},
			},
		},
		LocalVersions: &fakeLocalVersions{
			installed: map[string]string{
				"1.21.5": "/home/user/.govm/versions/go1.21.5",
			},
		},
		ActiveVersion: &fakeActiveVersion{
			active: "1.21.5",
		},
		Platform: Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	catalog, err := LoadVersionCatalog(ctx, deps)
	if err != nil {
		t.Fatalf("LoadVersionCatalog failed: %v", err)
	}

	if len(catalog.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(catalog.Versions))
	}

	if catalog.Versions[0].Version != "1.22.0" {
		t.Errorf("expected first version 1.22.0, got %s", catalog.Versions[0].Version)
	}
	if catalog.Versions[0].Installed {
		t.Errorf("expected 1.22.0 not installed")
	}

	if catalog.Versions[1].Version != "1.21.5" {
		t.Errorf("expected second version 1.21.5, got %s", catalog.Versions[1].Version)
	}
	if !catalog.Versions[1].Installed {
		t.Errorf("expected 1.21.5 installed")
	}
	if !catalog.Versions[1].Active {
		t.Errorf("expected 1.21.5 active")
	}

	if catalog.ActiveVersion != "1.21.5" {
		t.Errorf("expected active version 1.21.5, got %s", catalog.ActiveVersion)
	}
}

func TestLoadVersionCatalog_FallbackToExec(t *testing.T) {
	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			releases: []Release{
				{Version: "1.22.0", Stable: true, Files: []FileEntry{{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"}}},
			},
		},
		LocalVersions: &fakeLocalVersions{
			installed: map[string]string{},
		},
		ActiveVersion: &fakeActiveVersion{
			active:       "",
			pathFallback: "1.20.3",
		},
		Platform: Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	catalog, err := LoadVersionCatalog(ctx, deps)
	if err != nil {
		t.Fatalf("LoadVersionCatalog failed: %v", err)
	}

	if catalog.ActiveVersion != "1.20.3" {
		t.Errorf("expected active version from exec fallback 1.20.3, got %s", catalog.ActiveVersion)
	}
}

func TestLoadVersionCatalog_ReleaseSourceError(t *testing.T) {
	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			err: errors.New("network timeout"),
		},
		LocalVersions: &fakeLocalVersions{
			installed: map[string]string{},
		},
		ActiveVersion: &fakeActiveVersion{
			active: "1.21.0",
		},
		Platform: Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	_, err := LoadVersionCatalog(ctx, deps)
	if err == nil {
		t.Fatal("expected error from release source, got nil")
	}
	if !errors.Is(err, errors.New("network timeout")) && err.Error() != "fetch releases: network timeout" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadVersionCatalog_LocalVersionsError(t *testing.T) {
	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			releases: []Release{},
		},
		LocalVersions: &fakeLocalVersions{
			err: errors.New("permission denied"),
		},
		ActiveVersion: &fakeActiveVersion{
			active: "1.21.0",
		},
		Platform: Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	_, err := LoadVersionCatalog(ctx, deps)
	if err == nil {
		t.Fatal("expected error from local versions, got nil")
	}
	if err.Error() != "scan installed: permission denied" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadVersionCatalog_ActiveVersionError(t *testing.T) {
	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			releases: []Release{},
		},
		LocalVersions: &fakeLocalVersions{
			installed: map[string]string{},
		},
		ActiveVersion: &fakeActiveVersion{
			readErr: errors.New("corrupted file"),
		},
		Platform: Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	_, err := LoadVersionCatalog(ctx, deps)
	if err == nil {
		t.Fatal("expected error from active version, got nil")
	}
	if err.Error() != "read active version: corrupted file" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildVersionCatalog_Sorting(t *testing.T) {
	releases := []Release{
		{Version: "1.20.0", Stable: true, Files: []FileEntry{{Filename: "go1.20.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"}}},
		{Version: "1.22.1", Stable: true, Files: []FileEntry{{Filename: "go1.22.1.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"}}},
		{Version: "1.21.5", Stable: true, Files: []FileEntry{{Filename: "go1.21.5.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"}}},
	}

	versions := buildVersionCatalog(releases, "linux", "amd64", map[string]string{}, "")

	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}

	expected := []string{"1.22.1", "1.21.5", "1.20.0"}
	for i, exp := range expected {
		if versions[i].Version != exp {
			t.Errorf("expected versions[%d] = %s, got %s", i, exp, versions[i].Version)
		}
	}
}

func TestBuildVersionCatalogWithSource_UsesMirrorArchiveURL(t *testing.T) {
	releases := []Release{{
		Version: "1.22.1",
		Files: []FileEntry{{
			Filename: "go1.22.1.linux-amd64.tar.gz",
			OS:       "linux",
			Arch:     "amd64",
		}},
	}}

	versions := buildVersionCatalogWithSource(
		releases,
		"linux",
		"amd64",
		nil,
		"",
		"https://mirror.example/go/",
	)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if got := versions[0].URL; got != "https://mirror.example/go/go1.22.1.linux-amd64.tar.gz" {
		t.Fatalf("archive URL = %q, want mirror URL", got)
	}
}

func TestBuildVersionCatalog_PlatformFiltering(t *testing.T) {
	releases := []Release{
		{
			Version: "1.22.0",
			Stable:  true,
			Files: []FileEntry{
				{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64"},
				{Filename: "go1.22.0.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64"},
				{Filename: "go1.22.0.windows-amd64.zip", OS: "windows", Arch: "amd64"},
			},
		},
	}

	versions := buildVersionCatalog(releases, "darwin", "arm64", map[string]string{}, "")

	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0].Filename != "go1.22.0.darwin-arm64.tar.gz" {
		t.Errorf("expected darwin-arm64 file, got %s", versions[0].Filename)
	}
}

func TestLoadVersionCatalog_WithRealLocalAdapters(t *testing.T) {
	tmpDir := t.TempDir()
	versionsDir := filepath.Join(tmpDir, ".govm", "versions")
	activeFile := filepath.Join(tmpDir, ".govm", "active_version")

	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatalf("failed to create versions dir: %v", err)
	}

	version1Dir := filepath.Join(versionsDir, "go1.21.5")
	binDir := filepath.Join(version1Dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	goBin := filepath.Join(binDir, "go")
	if err := os.WriteFile(goBin, []byte("fake go binary"), 0755); err != nil {
		t.Fatalf("failed to create go binary: %v", err)
	}

	if err := os.WriteFile(activeFile, []byte("1.21.5"), 0644); err != nil {
		t.Fatalf("failed to write active version: %v", err)
	}

	resolver := &paths.Resolver{
		HomeDir: func() (string, error) {
			return tmpDir, nil
		},
	}

	deps := Dependencies{
		ReleaseSource: &fakeReleaseSource{
			releases: []Release{
				{
					Version: "1.22.0",
					Stable:  true,
					Files: []FileEntry{
						{Filename: "go1.22.0.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", SHA256: "abc123", Size: 1024},
					},
				},
				{
					Version: "1.21.5",
					Stable:  true,
					Files: []FileEntry{
						{Filename: "go1.21.5.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", SHA256: "def456", Size: 2048},
					},
				},
			},
		},
		LocalVersions: local.NewVersionScanner(resolver),
		ActiveVersion: local.NewActiveReader(resolver),
		Platform:      Platform{OS: "linux", Arch: "amd64"},
	}

	ctx := context.Background()
	catalog, err := LoadVersionCatalog(ctx, deps)
	if err != nil {
		t.Fatalf("LoadVersionCatalog with real adapters failed: %v", err)
	}

	if len(catalog.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(catalog.Versions))
	}

	var installed, active int
	for _, v := range catalog.Versions {
		if v.Installed {
			installed++
		}
		if v.Active {
			active++
		}
	}

	if installed != 1 {
		t.Errorf("expected 1 installed version, got %d", installed)
	}

	if active != 1 {
		t.Errorf("expected 1 active version, got %d", active)
	}

	if catalog.ActiveVersion != "1.21.5" {
		t.Errorf("expected active version 1.21.5, got %s", catalog.ActiveVersion)
	}

	for _, v := range catalog.Versions {
		if v.Version == "1.21.5" {
			if !v.Installed {
				t.Errorf("expected 1.21.5 to be installed")
			}
			if !v.Active {
				t.Errorf("expected 1.21.5 to be active")
			}
			if v.Path != version1Dir {
				t.Errorf("expected path %s, got %s", version1Dir, v.Path)
			}
		}
	}
}
