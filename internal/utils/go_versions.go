package utils

// This file owns everything related to the Go versions that govm
// manages: the GoVersion type, fetching the catalog from go.dev,
// comparing and sorting versions, and shim/PATH helpers. The
// version of the govm binary itself lives in govm_version.go.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

type GoVersion struct {
	Version   string
	Filename  string
	URL       string
	Installed bool
	Active    bool
	Path      string
	Stable    bool
}

// DisplayDescription returns the human-readable description shown for
// this version in the Available Versions list. Centralising the format
// here keeps the TUI list, benchmarks and test fixtures in sync.
func (v GoVersion) DisplayDescription() string {
	return "go" + v.Version + " " + v.Filename
}

// CompareGoVersions compares two Go version strings segment by segment.
// Segments are parsed as integers; non-numeric segments are treated as 0.
// A shorter version is considered lesser when it is a prefix of the
// longer one. Returns -1, 0, or 1.
func CompareGoVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	n := len(parts1)
	if len(parts2) > n {
		n = len(parts2)
	}
	for i := 0; i < n; i++ {
		p1 := 0
		p2 := 0
		if i < len(parts1) {
			p1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			p2, _ = strconv.Atoi(parts2[i])
		}
		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}
	return 0
}

// SortGoVersionsDesc sorts the slice in place so the highest version
// comes first.
func SortGoVersionsDesc(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return CompareGoVersions(versions[i], versions[j]) > 0
	})
}

func sortGoVersionRecordsDesc(records []GoVersion) {
	sort.SliceStable(records, func(i, j int) bool {
		return CompareGoVersions(records[i].Version, records[j].Version) > 0
	})
}

// FindLatestGoVersion returns the highest version in versions that
// matches query. If query is an exact element, that element wins.
// Otherwise the function falls back to the highest version that has
// query as a numeric prefix (with or without a separating dot). The
// bool result is false when no candidate matches.
func FindLatestGoVersion(versions []string, query string) (string, bool) {
	query = NormalizeGoVersionQuery(query)
	if query == "" {
		return "", false
	}

	best := ""
	bestOK := false
	for _, v := range versions {
		if v == query {
			return v, true
		}
		if !hasMatchingPrefix(v, query) {
			continue
		}
		if !bestOK || CompareGoVersions(v, best) > 0 {
			best = v
			bestOK = true
		}
	}
	return best, bestOK
}

func hasMatchingPrefix(version, query string) bool {
	if strings.HasPrefix(version, query+".") {
		return true
	}
	if strings.HasPrefix(version, query) {
		// Match "1" against "1.22.1" but not against "1.2.x" without
		// a dot, to avoid overlapping majors.
		if len(version) == len(query) {
			return true
		}
		next := version[len(query)]
		return next == '.'
	}
	return false
}

// NormalizeGoVersionQuery strips a leading "go" or "v" and trims
// surrounding whitespace so user input can be compared directly to
// the version strings produced by go.dev.
func NormalizeGoVersionQuery(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "go")
	s = strings.TrimPrefix(s, "v")
	return s
}

func SetupShimDirectory() error {
	resolver := paths.New()
	root, err := resolver.RootDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("failed to create govm directory: %v", err)
	}
	shimDir, err := resolver.ShimDir()
	if err != nil {
		return fmt.Errorf("failed to resolve shim directory: %v", err)
	}
	if err := os.MkdirAll(shimDir, 0755); err != nil {
		return fmt.Errorf("failed to create shim directory: %v", err)
	}
	return nil
}

// IsShimInPath reports whether the govm shim directory is present
// in the current PATH.
func IsShimInPath() bool {
	resolver := paths.New()
	shimDir, err := resolver.ShimDir()
	if err != nil {
		return false
	}
	currentPath := os.Getenv("PATH")
	pathSeparator := string(os.PathListSeparator)
	pathEntries := strings.Split(currentPath, pathSeparator)
	for _, entry := range pathEntries {
		if entry == shimDir {
			return true
		}
	}
	return false
}

func GetShimPathInstructions() string {
	if runtime.GOOS == "windows" {
		return "Add to PATH: %USERPROFILE%\\.govm\\shim"
	}
	return "Add to your shell config: export PATH=\"$HOME/.govm/shim:$PATH\""
}

// Doer is the minimal HTTP contract FetchGoVersions needs. It is
// satisfied by *http.Client and by httptest-backed fakes.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// goDevFile mirrors the per-file entry in go.dev's /dl/?mode=json
// payload. The Size field is intentionally omitted: govm never
// uses it.
type goDevFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// goDevRelease mirrors a release entry in go.dev's /dl/?mode=json
// payload. Version is normalised at the fetch boundary: the "go"
// prefix sent by go.dev is stripped in fetchGoDevReleases, so
// downstream code works with bare version strings ("1.22.0",
// not "go1.22.0").
type goDevRelease struct {
	Version string      `json:"version"`
	Stable  bool        `json:"stable"`
	Files   []goDevFile `json:"files"`
}

// fetchGoDevReleases downloads and decodes the go.dev release
// catalog. The caller owns the client (and its timeout). The
// returned releases carry versions with the leading "go" prefix
// already stripped.
func fetchGoDevReleases(client Doer, url string) ([]goDevRelease, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch go.dev releases: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch go.dev releases: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch go.dev releases: %w", err)
	}
	var releases []goDevRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("fetch go.dev releases: %w", err)
	}
	for i := range releases {
		releases[i].Version = strings.TrimPrefix(releases[i].Version, "go")
	}
	return releases, nil
}

func FetchGoVersions() tea.Msg {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	releases, err := fetchGoDevReleases(client, "https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return ErrMsg(err)
	}
	currentOS := runtime.GOOS
	arch := runtime.GOARCH
	resolver := paths.New()
	goVersionsDir, err := resolver.VersionsDir()
	if err != nil {
		return ErrMsg(err)
	}
	err = os.MkdirAll(goVersionsDir, 0755)
	if err != nil {
		return ErrMsg(err)
	}
	activeVersion := ""
	activeVersionFile, err := resolver.ActiveVersionFile()
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	} else {
		activeVersion = GetCurrentGoVersion()
	}
	installedVersions, err := ScanInstalledVersions(goVersionsDir)
	if err != nil {
		return ErrMsg(err)
	}
	var versions []GoVersion
	for _, release := range releases {
		for _, file := range release.Files {
			if file.OS == currentOS && file.Arch == arch {
				v := GoVersion{
					Version:   release.Version,
					Filename:  file.Filename,
					URL:       "https://go.dev/dl/" + file.Filename,
					Installed: false,
					Active:    false,
					Stable:    release.Stable,
				}
				if path, ok := installedVersions[release.Version]; ok {
					v.Installed = true
					v.Path = path
				}
				if activeVersion == release.Version {
					v.Active = true
				}
				versions = append(versions, v)
				break
			}
		}
	}
	sortGoVersionRecordsDesc(versions)
	return VersionsMsg(versions)
}

// ScanInstalledVersions walks the govm versions directory and returns
// the versions that are usable. An entry must:
//   - be a directory whose name starts with "go" followed by a
//     non-empty version segment,
//   - be a direct child of goVersionsDir (guards against path
//     traversal via symlinked entries),
//   - contain a bin/go binary (the entry point govm activates).
//
// The returned map is keyed by the bare version string (no "go"
// prefix) and valued by the absolute path to the version directory.
//
// ScanInstalledVersions is exported so the CLI shares the same disk
// view as the TUI (W-fix for candidate 9): previously cli.go had a
// parallel copy of this logic.
func ScanInstalledVersions(goVersionsDir string) (map[string]string, error) {
	entries, err := os.ReadDir(goVersionsDir)
	if err != nil {
		return nil, fmt.Errorf("scan installed versions: %w", err)
	}
	installed := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "go") {
			continue
		}
		versionStr := strings.TrimPrefix(entry.Name(), "go")
		if versionStr == "" {
			continue
		}
		path := filepath.Join(goVersionsDir, entry.Name())
		if !paths.IsDirectChild(goVersionsDir, path) {
			continue
		}
		goBin := filepath.Join(path, "bin", "go")
		if _, err := os.Stat(goBin); err != nil {
			continue
		}
		installed[versionStr] = path
	}
	return installed, nil
}

func GetCurrentGoVersion() string {
	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	parts := strings.Split(string(output), " ")
	if len(parts) >= 3 {
		return strings.TrimPrefix(parts[2], "go")
	}
	return ""
}
