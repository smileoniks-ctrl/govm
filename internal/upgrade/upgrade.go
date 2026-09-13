// Package upgrade decides whether a newer govm release exists than the
// running binary. It owns the domain terms Release build, Latest
// release and Upgrade notice (see CONTEXT.md) and depends on nothing
// but a source that reports the Latest release tag.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// LatestReleaseSource reports the tag of the Latest release: the newest
// stable govm release published for the project. Implementations live
// in the adapter layer; this package never performs I/O itself.
type LatestReleaseSource interface {
	LatestRelease(ctx context.Context) (string, error)
}

// Notice is the Upgrade notice payload: the Latest release that is
// newer than the running binary, in canonical "vX.Y.Z" form.
type Notice struct {
	Latest string
}

// Checker compares the running govm version against the Latest release.
type Checker struct {
	source  LatestReleaseSource
	current string
}

// NewChecker binds a Checker to the Latest release source and the
// version string of the running binary (as reported by
// utils.GetVersion).
func NewChecker(source LatestReleaseSource, current string) *Checker {
	return &Checker{source: source, current: current}
}

// Check asks the source for the Latest release and reports whether it
// is newer than the running binary. The bool is true only when a
// Notice should be shown. When the running binary is not a Release
// build the source is never consulted and (Notice{}, false, nil) is
// returned. Source failures are returned as errors; callers decide
// how loudly to react.
func (c *Checker) Check(ctx context.Context) (Notice, bool, error) {
	if c == nil || c.source == nil {
		return Notice{}, false, errors.New("no latest release source configured")
	}
	current, ok := ReleaseBuild(c.current)
	if !ok {
		return Notice{}, false, nil
	}
	tag, err := c.source.LatestRelease(ctx)
	if err != nil {
		return Notice{}, false, fmt.Errorf("latest release: %w", err)
	}
	latest, newer := Newer(current, tag)
	if !newer {
		return Notice{}, false, nil
	}
	return Notice{Latest: latest}, true, nil
}

// ReleaseBuild reports whether version identifies a Release build: a
// semantic version with or without the leading "v", as produced by a
// tagged GoReleaser build ("0.2.4") or by go install of a tagged module
// ("v0.2.4"). Pseudo-versions and pre-releases are valid semver and so
// count as Release builds; commit hashes, "(devel)" and "dev" do not.
// The returned string always carries the "v" prefix.
func ReleaseBuild(version string) (string, bool) {
	normalized := normalize(version)
	if !semver.IsValid(normalized) {
		return "", false
	}
	return normalized, true
}

// Newer reports whether latest is a valid semantic version strictly
// greater than current. Both inputs accept an optional "v" prefix; the
// returned latest carries it. An invalid input on either side yields
// false: an unparsable tag is never worth a notice.
func Newer(current, latest string) (string, bool) {
	cur, ok := ReleaseBuild(current)
	if !ok {
		return "", false
	}
	lat, ok := ReleaseBuild(latest)
	if !ok {
		return "", false
	}
	if semver.Compare(lat, cur) <= 0 {
		return "", false
	}
	return lat, true
}

func normalize(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}
