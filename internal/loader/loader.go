package loader

import (
	"context"
	"fmt"
	"strings"

	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// LoadVersionCatalog orchestrates the complete version-loading flow:
// fetch go.dev releases, scan installed versions, determine active
// version (with exec fallback), and merge everything into a single
// domain result. All I/O dependencies are passed explicitly so the
// orchestration logic is independently testable.
//
// The function fails fast on any critical dependency error. Callers
// control timeout and cancellation through ctx.
func LoadVersionCatalog(ctx context.Context, deps Dependencies) (VersionCatalog, error) {
	releases, err := deps.ReleaseSource.FetchReleases(ctx)
	if err != nil {
		return VersionCatalog{}, fmt.Errorf("fetch releases: %w", err)
	}

	installed, err := deps.LocalVersions.ScanInstalled(ctx)
	if err != nil {
		return VersionCatalog{}, fmt.Errorf("scan installed: %w", err)
	}

	activeVersion, err := deps.ActiveVersion.ReadActive(ctx)
	if err != nil {
		return VersionCatalog{}, fmt.Errorf("read active version: %w", err)
	}
	if activeVersion == "" {
		activeVersion = deps.ActiveVersion.GetFromPath(ctx)
	}

	versions := buildVersionCatalogWithSource(
		releases,
		deps.Platform.OS,
		deps.Platform.Arch,
		installed,
		activeVersion,
		deps.DistributionSource,
	)

	return VersionCatalog{
		Versions:      versions,
		ActiveVersion: activeVersion,
		Platform:      deps.Platform,
	}, nil
}

// buildVersionCatalog merges go.dev releases with the local view of
// installed and active versions. For each release, the first file
// matching the target OS/Arch wins. The returned slice is sorted
// highest-version-first.
//
// This is the same pure merge logic previously in utils, extracted
// here as a private implementation detail of the loader.
func buildVersionCatalog(
	releases []Release,
	targetOS, arch string,
	installed map[string]string,
	activeVersion string,
) []utils.GoVersion {
	return buildVersionCatalogWithSource(releases, targetOS, arch, installed, activeVersion, config.DefaultDistributionSource)
}

func buildVersionCatalogWithSource(
	releases []Release,
	targetOS, arch string,
	installed map[string]string,
	activeVersion, source string,
) []utils.GoVersion {
	var versions []utils.GoVersion
	for _, release := range releases {
		for _, file := range release.Files {
			if file.OS != targetOS || file.Arch != arch || (file.Kind != "" && file.Kind != "archive") {
				continue
			}
			v := utils.GoVersion{
				Version:  release.Version,
				Filename: file.Filename,
				URL:      archiveURL(source, file.Filename),
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
	sortGoVersionRecordsDesc(versions)
	return versions
}

func archiveURL(source, filename string) string {
	if source == "" {
		source = config.DefaultDistributionSource
	}
	return strings.TrimRight(source, "/") + "/" + filename
}

func sortGoVersionRecordsDesc(records []utils.GoVersion) {
	// Delegate to utils.CompareGoVersions for consistency with existing
	// version comparison logic throughout the codebase.
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if utils.CompareGoVersions(records[i].Version, records[j].Version) < 0 {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
}
