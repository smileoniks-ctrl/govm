package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/adapter/godev"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/state"
)

// sourceTimeout bounds the release-catalog probe. A variable so tests
// can shorten it; production never changes it.
var sourceTimeout = 5 * time.Second

// resolveSource returns the distribution source the doctor probes:
// the injected override, else the (normalised) settings.json value,
// else the go.dev default. settings.json problems are reported by
// checkSettings, so this never fails.
func resolveSource(deps Deps, settingsFile string) string {
	if deps.Source != "" {
		return deps.Source
	}
	settings := config.DefaultSettings()
	if data, err := deps.FS.ReadFile(settingsFile); err == nil {
		// Invalid JSON leaves the defaults in place.
		_ = json.Unmarshal(data, &settings)
	}
	return config.Normalize(settings).DistributionSource
}

func checkSource(ctx context.Context, e *env) Check {
	const name = CheckSource
	source := resolveSource(e.deps, e.settingsFile)
	if e.deps.Offline {
		return Check{
			Name:    name,
			Verdict: VerdictOK,
			Detail:  fmt.Sprintf("source %s skipped (--offline)", source),
		}
	}
	warn := func(reason string) Check {
		return Check{
			Name:    name,
			Verdict: VerdictWarn,
			Detail:  fmt.Sprintf("source %s unreachable: %s", source, reason),
			Hint:    "check network or run with --offline",
		}
	}
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	releases, err := godev.NewClientForSource(e.deps.HTTPClient, source).FetchReleases(ctx)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return warn(fmt.Sprintf("timeout after %v", sourceTimeout))
	case err != nil:
		return warn(err.Error())
	case len(releases) == 0:
		return warn("empty release list")
	}
	return Check{
		Name:    name,
		Verdict: VerdictOK,
		Detail:  fmt.Sprintf("source %s reachable (%d releases)", source, len(releases)),
	}
}

// diskUsage builds the production DiskUsage seam over resolver. The
// prune service is constructed lazily and only its read-only
// DiskUsage is called: no state lock, no recovery, no writes.
func diskUsage(resolver *paths.Resolver) func(context.Context) (prune.Summary, error) {
	return func(ctx context.Context) (prune.Summary, error) {
		coordinator := state.NewCoordinator(resolver)
		lifecycleSvc, err := lifecycle.New(resolver, coordinator)
		if err != nil {
			return prune.Summary{}, err
		}
		pruneSvc, err := prune.New(resolver, coordinator, lifecycleSvc)
		if err != nil {
			return prune.Summary{}, err
		}
		return pruneSvc.DiskUsage(ctx)
	}
}

func checkDisk(ctx context.Context, e *env) Check {
	const name = CheckDisk
	if e.layoutErr != nil {
		return layoutFailure(name, e.layoutErr)
	}
	summary, err := e.deps.DiskUsage(ctx)
	if err != nil {
		return Check{
			Name:    name,
			Verdict: VerdictWarn,
			Detail:  fmt.Sprintf("disk usage unavailable: %v", err),
			Hint:    fmt.Sprintf("check permissions on %s", e.root),
		}
	}
	// Inactive versions are counted against a valid active version
	// only: without one prune preserves every toolchain, so the prune
	// hint would be misleading.
	inactive := 0
	if e.active != "" {
		for v := range summary.VersionBytes {
			if v != e.active {
				inactive++
			}
		}
	}
	detail := fmt.Sprintf("disk: %s (%s%s), downloads %s",
		plural(len(summary.VersionBytes), "version"),
		prune.FormatBytes(summary.InstalledBytes),
		inactiveSuffix(inactive),
		prune.FormatBytes(summary.DownloadBytes),
	)
	if len(summary.Warnings) > 0 {
		detail += "; warnings: " + joinWarnings(summary.Warnings)
	}
	if inactive > 0 || summary.DownloadBytes > 0 {
		return Check{
			Name:    name,
			Verdict: VerdictWarn,
			Detail:  detail,
			Hint:    "run `govm prune` to remove inactive versions and downloads",
		}
	}
	return Check{Name: name, Verdict: VerdictOK, Detail: detail}
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func inactiveSuffix(inactive int) string {
	if inactive == 0 {
		return ""
	}
	return fmt.Sprintf(", %d inactive", inactive)
}

func joinWarnings(warnings []prune.Warning) string {
	parts := make([]string, 0, len(warnings))
	for _, w := range warnings {
		parts = append(parts, w.Error())
	}
	return strings.Join(parts, "; ")
}
