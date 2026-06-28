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

	tea "charm.land/bubbletea/v2"
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	govmDir := filepath.Join(homeDir, ".govm")
	if err := os.MkdirAll(govmDir, 0755); err != nil {
		return fmt.Errorf("failed to create govm directory: %v", err)
	}
	shimDir := filepath.Join(govmDir, "shim")
	if err := os.MkdirAll(shimDir, 0755); err != nil {
		return fmt.Errorf("failed to create shim directory: %v", err)
	}
	return nil
}

// IsShimInPath reports whether the govm shim directory is present
// in the current PATH.
func IsShimInPath() bool {
	homeDir, _ := os.UserHomeDir()
	shimDir := filepath.Join(homeDir, ".govm", "shim")
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

func FetchGoVersions() tea.Msg {
	// I randomly put 10 second here
	client := &http.Client{
		Timeout: 10 * 1000000000,
	}
	resp, err := client.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return ErrMsg(fmt.Errorf("failed to connect to go.dev: %v", err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrMsg(err)
	}
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
		Files   []struct {
			Filename string `json:"filename"`
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Size     int    `json:"size"`
		} `json:"files"`
	}
	err = json.Unmarshal(body, &releases)
	if err != nil {
		return ErrMsg(fmt.Errorf("failed to parse API response: %v", err))
	}
	currentOS := runtime.GOOS
	arch := runtime.GOARCH
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ErrMsg(err)
	}
	goVersionsDir := filepath.Join(homeDir, ".govm", "versions")
	err = os.MkdirAll(goVersionsDir, 0755)
	if err != nil {
		return ErrMsg(err)
	}
	activeVersion := ""
	activeVersionFile := filepath.Join(homeDir, ".govm", "active_version")
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	} else {
		activeVersion = GetCurrentGoVersion()
	}
	installedVersions := map[string]string{}
	entries, _ := os.ReadDir(goVersionsDir)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "go") {
			versionPath := filepath.Join(goVersionsDir, entry.Name())
			version := strings.TrimPrefix(entry.Name(), "go")
			goBin := filepath.Join(versionPath, "bin", "go")
			if _, err := os.Stat(goBin); err == nil {
				installedVersions[version] = versionPath
			}
		}
	}
	var versions []GoVersion
	for _, release := range releases {
		version := strings.TrimPrefix(release.Version, "go")
		for _, file := range release.Files {
			if file.OS == currentOS && file.Arch == arch {
				v := GoVersion{
					Version:   version,
					Filename:  file.Filename,
					URL:       "https://go.dev/dl/" + file.Filename,
					Installed: false,
					Active:    false,
					Stable:    release.Stable,
				}
				if path, ok := installedVersions[version]; ok {
					v.Installed = true
					v.Path = path
				}
				if activeVersion == version {
					v.Active = true
				}
				versions = append(versions, v)
				break
			}
		}
	}
	// Sort using the shared helper instead of the previous inline logic
	// so that the CLI matching helpers and the TUI list stay aligned.
	stringsOnly := make([]string, len(versions))
	for i, v := range versions {
		stringsOnly[i] = v.Version
	}
	SortGoVersionsDesc(stringsOnly)
	order := make(map[string]int, len(stringsOnly))
	for i, v := range stringsOnly {
		order[v] = i
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return order[versions[i].Version] < order[versions[j].Version]
	})
	return VersionsMsg(versions)
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
