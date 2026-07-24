package utils

// This file owns everything related to the Go versions that govm
// manages: the GoVersion type, fetching the catalog from go.dev,
// comparing and sorting versions, and shim/PATH helpers. The
// version of the govm binary itself lives in govm_version.go.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	Version  string
	Filename string
	URL      string
	// SHA256 is the archive checksum published by go.dev. It is empty
	// for releases where go.dev did not publish one; callers may treat
	// the absence as a warning rather than a hard failure.
	SHA256 string
	// Size is the archive size in bytes published by go.dev.
	Size      int64
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
// payload. SHA256 and Size are propagated into GoVersion so the
// install core can verify archive integrity.
type goDevFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kind     string `json:"kind"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
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

// buildVersionCatalog builds the user-facing version list by joining
// the go.dev release catalog with the local view of installed and
// active versions. For each release, the first file matching the
// current OS/Arch wins (consistent with the original behaviour). The
// returned slice is sorted highest-version-first by the caller.
//
// The function is pure: no I/O, no clocks, no global state. That
// makes the merge logic (release selection, installed/active flags,
// URL construction) independently testable.
func buildVersionCatalog(
	releases []goDevRelease,
	currentOS, arch string,
	installed map[string]string,
	activeVersion string,
) []GoVersion {
	var versions []GoVersion
	for _, release := range releases {
		for _, file := range release.Files {
			if file.OS != currentOS || file.Arch != arch || (file.Kind != "" && file.Kind != "archive") {
				continue
			}
			v := GoVersion{
				Version:  release.Version,
				Filename: file.Filename,
				URL:      "https://go.dev/dl/" + file.Filename,
				SHA256:   file.SHA256,
				Size:     file.Size,
				Stable:   release.Stable,
			}
			if path, ok := installed[release.Version]; ok {
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
	return versions
}

func FetchGoVersions() tea.Msg {
	client := &http.Client{Timeout: 10 * time.Second}
	releases, err := fetchGoDevReleases(client, "https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return ErrMsg(err)
	}

	resolver := paths.New()
	goVersionsDir, err := resolver.VersionsDir()
	if err != nil {
		return ErrMsg(err)
	}
	if err := ensureVersionsDir(goVersionsDir); err != nil {
		return ErrMsg(err)
	}
	installed, err := ScanInstalledVersions(goVersionsDir)
	if err != nil {
		return ErrMsg(err)
	}

	activeVersionFile, err := resolver.ActiveVersionFile()
	if err != nil {
		return ErrMsg(err)
	}
	activeVersion, err := readActiveVersion(activeVersionFile)
	if err != nil {
		return ErrMsg(err)
	}
	if activeVersion == "" {
		// Fresh install or no active version recorded yet: fall back
		// to whatever go is on PATH so the catalog can still mark the
		// active toolchain. The exec-fallback stays in the orchestrator
		// (decision ii) so readActiveVersion remains a pure disk
		// function.
		activeVersion = GetCurrentGoVersion()
	}

	versions := buildVersionCatalog(releases, runtime.GOOS, runtime.GOARCH, installed, activeVersion)
	sortGoVersionRecordsDesc(versions)
	return VersionsMsg(versions)
}

// ensureVersionsDir guarantees that the govm versions directory
// exists. ScanInstalledVersions treats a missing directory as an
// error rather than as an empty result, so callers must create the
// directory first. Extracted from FetchGoVersions so the write
// side-effect is named rather than buried in the read path.
func ensureVersionsDir(goVersionsDir string) error {
	if err := os.MkdirAll(goVersionsDir, 0755); err != nil {
		return fmt.Errorf("ensure govm versions dir: %w", err)
	}
	return nil
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

// readActiveVersion reads the govm active-version file. It returns:
//   - ("", nil) when the file does not exist (fresh install). The
//     orchestrator decides what to do, typically falling back to the
//     go binary found on PATH.
//   - ("", err) on any other read failure (e.g. permission denied).
//     This surfaces latent bug (I): previously the outer err from
//     Resolver.ActiveVersionFile was shadowed and swallowed, masking
//     permission errors as "no active version" and silently falling
//     back to a possibly-unrelated system go.
//   - (version, nil) on success. The version string is the raw file
//     contents; no normalisation is applied.
func readActiveVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read active version file: %w", err)
	}
	return string(data), nil
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
