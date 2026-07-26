// Package prune removes inactive, valid Go toolchains and govm-owned
// interrupted download files.
package prune

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/state"
	"github.com/smileoniks-ctrl/govm/internal/version"
)

const (
	versionPrefix = "go"
	partPrefix    = ".govm-install-"
	partSuffix    = ".part"
)

// Service is the core prune service.
type Service struct {
	resolver  *paths.Resolver
	state     *state.Coordinator
	lifecycle *lifecycle.Service
}

// Candidate describes a valid inactive toolchain or a safe temporary file.
type Candidate struct {
	Path    string
	Version string
	Bytes   int64
	Kind    CandidateKind
}

// CandidateKind identifies the object represented by Candidate.
type CandidateKind int

const (
	CandidateVersion CandidateKind = iota + 1
	CandidateDownload
)

// Result reports planned and completed cleanup, with per-object warnings.
type Result struct {
	Candidates []Candidate
	Removed    []Candidate
	Warnings   []Warning
}

// Warning is a non-fatal condition discovered while scanning or cleaning.
type Warning struct {
	Path string
	Err  error
}

func (w Warning) Error() string {
	if w.Err == nil {
		return w.Path
	}
	if w.Path == "" {
		return w.Err.Error()
	}
	return fmt.Sprintf("%s: %v", w.Path, w.Err)
}

// Summary contains logical disk usage and warnings.
type Summary struct {
	InstalledBytes   int64
	DownloadBytes    int64
	ReclaimableBytes int64
	VersionBytes     map[string]int64
	Warnings         []Warning
}

// New constructs a prune service using the supplied shared dependencies.
func New(resolver *paths.Resolver, coordinator *state.Coordinator, lifecycleService *lifecycle.Service) (*Service, error) {
	if resolver == nil {
		resolver = paths.New()
	}
	if coordinator == nil {
		return nil, errors.New("prune coordinator is nil")
	}
	if lifecycleService == nil {
		return nil, errors.New("prune lifecycle service is nil")
	}
	return &Service{resolver: resolver, state: coordinator, lifecycle: lifecycleService}, nil
}

// NewService is an explicit constructor alias for composition roots.
func NewService(resolver *paths.Resolver, coordinator *state.Coordinator, lifecycleService *lifecycle.Service) (*Service, error) {
	return New(resolver, coordinator, lifecycleService)
}

// Prune removes all valid inactive toolchains and safe temporary downloads.
// Invalid or unknown objects are preserved and reported as warnings.
func (s *Service) Prune(ctx context.Context) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result Result
	var operationErrors []error
	transactionOutstanding := false
	coordinated, err := s.state.Mutate(ctx, func(ctx context.Context, store *state.MarkerStore) error {
		plan, planErr := s.plan(ctx)
		if planErr != nil {
			return planErr
		}
		result.Candidates = plan.candidates
		result.Warnings = append(result.Warnings, plan.warnings...)
		if plan.activeErr != nil {
			result.Warnings = append(result.Warnings, Warning{Path: "active_version", Err: plan.activeErr})
			operationErrors = append(operationErrors, plan.activeErr)
		}
		for _, candidate := range plan.candidates {
			if err := ctx.Err(); err != nil {
				result.Warnings = append(result.Warnings, Warning{Path: candidate.Path, Err: err})
				operationErrors = append(operationErrors, err)
				continue
			}
			if candidate.Kind == CandidateVersion {
				if transactionOutstanding {
					skippedErr := errors.New("skipped while a previous deletion transaction requires recovery")
					result.Warnings = append(result.Warnings, Warning{Path: candidate.Path, Err: skippedErr})
					operationErrors = append(operationErrors, skippedErr)
					continue
				}
				removed, deleteErr := s.lifecycle.DeleteLocked(ctx, store, candidate.Version)
				if deleteErr != nil {
					result.Warnings = append(result.Warnings, Warning{Path: candidate.Path, Err: deleteErr})
					operationErrors = append(operationErrors, deleteErr)
					continue
				}
				for _, warning := range removed.Warnings {
					result.Warnings = append(result.Warnings, Warning{Path: candidate.Path, Err: warning})
					operationErrors = append(operationErrors, warning)
					transactionOutstanding = true
				}
			} else if removeErr := os.Remove(candidate.Path); removeErr != nil {
				result.Warnings = append(result.Warnings, Warning{Path: candidate.Path, Err: removeErr})
				operationErrors = append(operationErrors, removeErr)
				continue
			}
			result.Removed = append(result.Removed, candidate)
		}
		return nil
	})
	result.Warnings = append(recoveryWarnings(coordinated.RecoveryWarnings), result.Warnings...)
	if err != nil {
		return result, err
	}
	return result, errors.Join(operationErrors...)
}

// Preview returns the cleanup plan while holding the shared state lock.
func (s *Service) Preview(ctx context.Context) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result Result
	coordinated, err := s.state.Mutate(ctx, func(ctx context.Context, _ *state.MarkerStore) error {
		plan, planErr := s.plan(ctx)
		if planErr != nil {
			return planErr
		}
		result.Candidates = plan.candidates
		result.Warnings = append(result.Warnings, plan.warnings...)
		if plan.activeErr != nil {
			result.Warnings = append(result.Warnings, Warning{Path: "active_version", Err: plan.activeErr})
			return plan.activeErr
		}
		return nil
	})
	result.Warnings = append(recoveryWarnings(coordinated.RecoveryWarnings), result.Warnings...)
	if err != nil {
		return result, err
	}
	return result, nil
}

// DiskUsage computes logical regular-file sizes. Hard links are counted once
// and symlinks are ignored; traversal failures produce approximate results.
func (s *Service) DiskUsage(ctx context.Context) (Summary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, err := s.resolver.VersionsDir()
	if err != nil {
		return Summary{}, err
	}
	downloads, err := s.resolver.DownloadsDir()
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{}
	summary.VersionBytes = make(map[string]int64)
	installed, installedWarnings := logicalSize(ctx, versions)
	summary.InstalledBytes = installed
	summary.Warnings = append(summary.Warnings, installedWarnings...)
	if entries, readErr := os.ReadDir(versions); readErr == nil {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), versionPrefix) {
				continue
			}
			canonical := strings.TrimPrefix(entry.Name(), versionPrefix)
			if version.Validate(canonical) != nil || s.lifecycle.ValidateInstalledVersion(canonical) != nil {
				continue
			}
			bytes, sizeWarnings := logicalSize(ctx, filepath.Join(versions, entry.Name()))
			summary.VersionBytes[canonical] = bytes
			summary.Warnings = append(summary.Warnings, sizeWarnings...)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		summary.Warnings = append(summary.Warnings, Warning{Path: versions, Err: readErr})
	}
	downloadSize, downloadWarnings := matchingDownloadSize(ctx, downloads)
	summary.DownloadBytes = downloadSize
	summary.Warnings = append(summary.Warnings, downloadWarnings...)
	plan, planErr := s.plan(ctx)
	if planErr != nil {
		return summary, planErr
	}
	for _, candidate := range plan.candidates {
		summary.ReclaimableBytes += candidate.Bytes
	}
	summary.Warnings = append(summary.Warnings, plan.warnings...)
	if plan.activeErr != nil {
		summary.Warnings = append(summary.Warnings, Warning{Path: "active_version", Err: plan.activeErr})
	}
	return summary, nil
}

// FormatBytes formats a byte count using binary units.
func FormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", bytes)
}

type cleanupPlan struct {
	candidates []Candidate
	warnings   []Warning
	activeErr  error
}

func (s *Service) plan(ctx context.Context) (cleanupPlan, error) {
	versions, err := s.resolver.VersionsDir()
	if err != nil {
		return cleanupPlan{}, err
	}
	downloads, err := s.resolver.DownloadsDir()
	if err != nil {
		return cleanupPlan{}, err
	}
	activePath, err := s.resolver.ActiveVersionFile()
	if err != nil {
		return cleanupPlan{}, err
	}
	active, activePresent, activeErr := readActive(activePath)
	if activePresent && activeErr == nil {
		if err := s.lifecycle.ValidateInstalledVersion(active); err != nil {
			activeErr = fmt.Errorf("active version %q is not installed: %w", active, err)
		}
	}
	plan := cleanupPlan{activeErr: activeErr}
	versionCandidates, versionWarnings := s.scanVersions(ctx, versions, active, activePresent, activeErr)
	downloadCandidates, downloadWarnings := scanDownloads(ctx, downloads)
	plan.candidates = append(versionCandidates, downloadCandidates...)
	plan.warnings = append(versionWarnings, downloadWarnings...)
	return plan, nil
}

func (s *Service) scanVersions(ctx context.Context, versionsDir, active string, activePresent bool, activeErr error) ([]Candidate, []Warning) {
	if warning := validateScanDirectory(versionsDir); warning != nil {
		return nil, []Warning{*warning}
	}
	entries, err := os.ReadDir(versionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Warning{{Path: versionsDir, Err: err}}
	}
	var candidates []Candidate
	var warnings []Warning
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, Warning{Path: versionsDir, Err: err})
			break
		}
		name := entry.Name()
		if !strings.HasPrefix(name, versionPrefix) {
			warnings = append(warnings, Warning{Path: filepath.Join(versionsDir, name), Err: errors.New("preserved unknown object")})
			continue
		}
		canonical := strings.TrimPrefix(name, versionPrefix)
		if err := version.Validate(canonical); err != nil {
			warnings = append(warnings, Warning{Path: filepath.Join(versionsDir, name), Err: fmt.Errorf("preserved unknown object: %w", err)})
			continue
		}
		if !activePresent || activeErr != nil || canonical == active {
			if canonical == active && activePresent && activeErr == nil {
				continue
			}
			if activeErr == nil && !activePresent {
				warnings = append(warnings, Warning{Path: filepath.Join(versionsDir, name), Err: errors.New("preserved because active version is unavailable")})
			}
			continue
		}
		if err := s.lifecycle.ValidateInstalledVersion(canonical); err != nil {
			warnings = append(warnings, Warning{Path: filepath.Join(versionsDir, name), Err: fmt.Errorf("preserved invalid toolchain: %w", err)})
			continue
		}
		bytes, sizeWarnings := logicalSize(ctx, filepath.Join(versionsDir, name))
		warnings = append(warnings, sizeWarnings...)
		candidates = append(candidates, Candidate{
			Path:    filepath.Join(versionsDir, name),
			Version: canonical,
			Bytes:   bytes,
			Kind:    CandidateVersion,
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, warnings
}

func scanDownloads(ctx context.Context, downloadsDir string) ([]Candidate, []Warning) {
	if warning := validateScanDirectory(downloadsDir); warning != nil {
		return nil, []Warning{*warning}
	}
	entries, err := os.ReadDir(downloadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Warning{{Path: downloadsDir, Err: err}}
	}
	var candidates []Candidate
	var warnings []Warning
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, Warning{Path: downloadsDir, Err: err})
			break
		}
		name := entry.Name()
		if !strings.HasPrefix(name, partPrefix) || !strings.HasSuffix(name, partSuffix) ||
			len(name) == len(partPrefix)+len(partSuffix) {
			warnings = append(warnings, Warning{Path: filepath.Join(downloadsDir, name), Err: errors.New("preserved unknown download object")})
			continue
		}
		path := filepath.Join(downloadsDir, name)
		info, infoErr := os.Lstat(path)
		if infoErr != nil {
			warnings = append(warnings, Warning{Path: path, Err: infoErr})
			continue
		}
		if !info.Mode().IsRegular() {
			warnings = append(warnings, Warning{Path: path, Err: errors.New("preserved non-regular temporary object")})
			continue
		}
		candidates = append(candidates, Candidate{Path: path, Bytes: info.Size(), Kind: CandidateDownload})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, warnings
}

func validateScanDirectory(path string) *Warning {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &Warning{Path: path, Err: err}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return &Warning{Path: path, Err: errors.New("scan path is not a real directory")}
	}
	return nil
}

func readActive(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, errors.New("active version marker is missing")
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false, errors.New("active version marker is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	value := string(data)
	if err := version.Validate(value); err != nil {
		return "", false, fmt.Errorf("invalid active version marker: %w", err)
	}
	return value, true, nil
}

func logicalSize(ctx context.Context, root string) (int64, []Warning) {
	var total int64
	var warnings []Warning
	var seen []fs.FileInfo
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			warnings = append(warnings, Warning{Path: path, Err: walkErr})
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			warnings = append(warnings, Warning{Path: path, Err: err})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if seenFile(info, seen) {
			return nil
		}
		seen = append(seen, info)
		total += info.Size()
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		warnings = append(warnings, Warning{Path: root, Err: err})
	}
	return total, warnings
}

func matchingDownloadSize(ctx context.Context, root string) (int64, []Warning) {
	if warning := validateScanDirectory(root); warning != nil {
		return 0, []Warning{*warning}
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, []Warning{{Path: root, Err: err}}
	}
	var total int64
	var warnings []Warning
	var seen []fs.FileInfo
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, Warning{Path: root, Err: err})
			break
		}
		if !strings.HasPrefix(entry.Name(), partPrefix) || !strings.HasSuffix(entry.Name(), partSuffix) ||
			len(entry.Name()) == len(partPrefix)+len(partSuffix) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			warnings = append(warnings, Warning{Path: path, Err: err})
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if seenFile(info, seen) {
			continue
		}
		seen = append(seen, info)
		total += info.Size()
	}
	return total, warnings
}

func seenFile(info fs.FileInfo, seen []fs.FileInfo) bool {
	for _, previous := range seen {
		if os.SameFile(info, previous) {
			return true
		}
	}
	return false
}

func recoveryWarnings(warnings []state.Warning) []Warning {
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, Warning{Path: "transaction", Err: errors.New(warning.Error())})
	}
	return result
}
