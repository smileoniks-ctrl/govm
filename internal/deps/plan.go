package deps

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// UpdateLevel bounds the target version of every entry in an update
// plan. The zero value is LevelLatest, which keeps the historical
// behaviour: the target is whatever `go list -m -u` reports as Latest.
type UpdateLevel int

const (
	LevelLatest UpdateLevel = iota
	LevelMinor
	LevelPatch
)

// Levels lists the update levels in the order the TUI cycles through
// them: most conservative first.
var Levels = []UpdateLevel{LevelPatch, LevelMinor, LevelLatest}

func (l UpdateLevel) String() string {
	switch l {
	case LevelLatest:
		return "latest"
	case LevelMinor:
		return "minor"
	case LevelPatch:
		return "patch"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// Label returns the capitalised form used in dialogs.
func (l UpdateLevel) Label() string {
	s := l.String()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ParseUpdateLevel parses a level name as used by the CLI flags.
func ParseUpdateLevel(name string) (UpdateLevel, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "latest", "":
		return LevelLatest, nil
	case "minor":
		return LevelMinor, nil
	case "patch":
		return LevelPatch, nil
	default:
		return LevelLatest, fmt.Errorf("unknown update level %q (want patch, minor or latest)", name)
	}
}

// UpdateSelection describes which modules an update plan should cover
// and how far each may move. Modules are queries resolved through
// ResolveModulePath (exact path or unique trailing segment suffix). An
// empty Modules list means "every direct dependency"; a named module
// may be indirect.
type UpdateSelection struct {
	Modules []string
	Level   UpdateLevel
}

// Explicit reports whether the selection names specific modules.
func (s UpdateSelection) Explicit() bool { return len(s.Modules) > 0 }

func cloneSelection(s UpdateSelection) UpdateSelection {
	out := s
	if s.Modules != nil {
		out.Modules = make([]string, len(s.Modules))
		copy(out.Modules, s.Modules)
	}
	return out
}

// UnknownModuleError is returned when a module query matches no
// dependency of the module.
type UnknownModuleError struct {
	Query string
}

func (e UnknownModuleError) Error() string {
	return fmt.Sprintf("unknown module %q", e.Query)
}

// AmbiguousModuleError is returned when a module query matches more
// than one dependency by suffix.
type AmbiguousModuleError struct {
	Query      string
	Candidates []string
}

func (e AmbiguousModuleError) Error() string {
	return fmt.Sprintf("ambiguous module %q matches: %s", e.Query, strings.Join(e.Candidates, ", "))
}

// ResolveModulePath maps a user-supplied module query to the path of
// exactly one dependency. An exact path wins; otherwise the query must
// match a unique trailing run of path segments ("cobra", "spf13/cobra").
func ResolveModulePath(deps []ModuleDependency, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", UnknownModuleError{Query: query}
	}
	var candidates []string
	for _, d := range deps {
		if d.Path == query {
			return d.Path, nil
		}
		if strings.HasSuffix(d.Path, "/"+query) {
			candidates = append(candidates, d.Path)
		}
	}
	switch len(candidates) {
	case 0:
		return "", UnknownModuleError{Query: query}
	case 1:
		return candidates[0], nil
	default:
		sort.Strings(candidates)
		return "", AmbiguousModuleError{Query: query, Candidates: candidates}
	}
}

// TargetVersion returns the version d should move to under level, or
// "" when no update within the level exists. LevelLatest returns the
// Latest reported by `go list -m -u`. Patch and minor pick the highest
// known version (from Versions) sharing the current major.minor or
// major respectively; pre-releases are candidates only when the
// current version is itself a pre-release. When Versions is empty
// (offline load) the level is checked against Latest alone.
func TargetVersion(d ModuleDependency, level UpdateLevel) string {
	if d.Error != "" || d.Version == "" {
		return ""
	}
	if level == LevelLatest {
		if d.Latest != "" && d.Latest != d.Version {
			return d.Latest
		}
		return ""
	}
	if len(d.Versions) == 0 {
		if d.Latest != "" && d.Latest != d.Version && withinLevel(d.Version, d.Latest, level) {
			return d.Latest
		}
		return ""
	}
	best := ""
	for _, v := range d.Versions {
		if !withinLevel(d.Version, v, level) {
			continue
		}
		if best == "" || semver.Compare(v, best) > 0 {
			best = v
		}
	}
	return best
}

// withinLevel reports whether candidate is a valid, newer version of
// current that stays inside level.
func withinLevel(current, candidate string, level UpdateLevel) bool {
	if !semver.IsValid(candidate) || !semver.IsValid(current) {
		return false
	}
	if semver.Compare(candidate, current) <= 0 {
		return false
	}
	if semver.Prerelease(candidate) != "" && semver.Prerelease(current) == "" {
		return false
	}
	switch level {
	case LevelPatch:
		return semver.MajorMinor(candidate) == semver.MajorMinor(current)
	case LevelMinor:
		return semver.Major(candidate) == semver.Major(current)
	default:
		return true
	}
}

// BuildUpdatePlan is the single policy point deciding which modules an
// update touches and which version each moves to. Without explicit
// modules the plan covers every direct dependency with an update at
// the level. With explicit modules each query is resolved (errors on
// unknown or ambiguous queries), indirect modules are allowed, and
// modules already at the level's target are silently omitted. Entries
// keep the order of first mention; duplicates collapse.
func BuildUpdatePlan(deps []ModuleDependency, sel UpdateSelection) ([]DependencyUpdateEntry, error) {
	var entries []DependencyUpdateEntry
	if !sel.Explicit() {
		for _, d := range deps {
			if d.Indirect {
				continue
			}
			if target := TargetVersion(d, sel.Level); target != "" {
				entries = append(entries, DependencyUpdateEntry{Path: d.Path, OldVersion: d.Version, NewVersion: target})
			}
		}
		return entries, nil
	}
	byPath := make(map[string]ModuleDependency, len(deps))
	for _, d := range deps {
		byPath[d.Path] = d
	}
	seen := make(map[string]bool, len(sel.Modules))
	for _, query := range sel.Modules {
		path, err := ResolveModulePath(deps, query)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		d := byPath[path]
		if target := TargetVersion(d, sel.Level); target != "" {
			entries = append(entries, DependencyUpdateEntry{Path: d.Path, OldVersion: d.Version, NewVersion: target})
		}
	}
	return entries, nil
}
