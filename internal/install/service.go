// Package install provides the transactional Go toolchain installer.
//
// The public Service performs an atomic install: download, integrity
// check, extract, validate the extracted toolchain, and swap it into
// place with rollback on failure. All side effects are coordinated
// through a cross-process lock and unique staging paths so a crash
// never leaves a half-installed version visible to the rest of govm.
package install

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

const (
	// stagingPrefix marks in-progress extraction directories created
	// under the versions directory. Orphans carrying this prefix are
	// removed at the start of every install.
	stagingPrefix = ".install-"
	// partPrefix + partSuffix mark in-progress downloads stored under
	// the downloads directory.
	partPrefix = ".govm-install-"
	partSuffix = ".part"
	// backupPrefix marks the temporary copy of a pre-existing install
	// held during the commit swap so it can be restored on failure.
	backupPrefix = ".govm-backup-"
	// recoveryPrefix marks a backup that could not be restored after a
	// failed commit. It is preserved on disk and never cleaned by govm.
	recoveryPrefix = ".recovery-"

	// dirMode is the restrictive mode used for every govm-owned
	// directory created during an install.
	dirMode = 0o700
	// fileMode is the restrictive mode used for the download part file.
	fileMode = 0o600
)

var versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:(?:\.[0-9]+)|(?:beta|rc)[0-9]+)?$`)

// httpDoer executes a single HTTP request below the redirect/policy
// layer. Production wires an *http.Client whose CheckRedirect enforces
// the official-host policy; tests inject a fake to bypass it.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// extractor unpacks archivePath into destination.
type extractor func(ctx context.Context, archivePath, destination string) error

// commandRunner runs the extracted toolchain's "go version" command
// and returns its combined output.
type commandRunner func(ctx context.Context, binary string) ([]byte, error)

// renamer renames old to new. It is os.Rename in production; tests
// inject failures to exercise the rollback paths deterministically.
type renamer func(old, new string) error

// remover removes a path tree. It is only used for post-commit
// cleanup, whose failures become non-fatal Result warnings.
// Failure-path cleanup always uses os.RemoveAll directly.
type remover func(path string) error

// Service is the transactional Go toolchain installer.
type Service struct {
	resolver *paths.Resolver
	doer     httpDoer
	extract  extractor
	verify   commandRunner
	rename   renamer
	cleanup  remover
}

// NewService returns a production-configured Service.
func NewService() *Service {
	return &Service{
		resolver: paths.New(),
		doer:     newProductionHTTPClient(),
		extract:  extractArchive,
		verify:   defaultCommandRunner,
		rename:   os.Rename,
		cleanup:  os.RemoveAll,
	}
}

// Install performs a transactional install of req.Version.
//
// Every failure path returns an *Error describing the stage; cleanup
// of the current staging/download artefacts is always attempted and
// any cleanup failure is joined onto the original error.
func (s *Service) Install(ctx context.Context, req Request) (Result, error) {
	if err := validateRequest(req); err != nil {
		return Result{}, &Error{Stage: StageValidate, Err: err}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, &Error{Stage: StageValidate, Err: err}
	}

	root, err := s.resolver.RootDir()
	if err != nil {
		return Result{}, &Error{Stage: StageLock, Err: err}
	}
	// The lock file lives under root, so root must exist before we try
	// to acquire it. MkdirAll is idempotent and safe across processes.
	if err := os.MkdirAll(root, dirMode); err != nil {
		return Result{}, &Error{Stage: StageLock, Err: fmt.Errorf("create root directory: %w", err)}
	}
	lockPath, err := s.resolver.InstallationLockFile()
	if err != nil {
		return Result{}, &Error{Stage: StageLock, Err: err}
	}
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return Result{}, &Error{Stage: StageLock, Err: fmt.Errorf("acquire install lock: %w", err)}
	}
	if !locked {
		// Contention: another process (or goroutine) holds the lock.
		// Fail fast rather than block.
		return Result{}, &Error{Stage: StageLock, Err: errors.New("another installation is in progress")}
	}
	defer lock.Unlock()
	// The lock file is intentionally never deleted; it persists for the
	// lifetime of the govm root.

	versionsDir, err := s.resolver.VersionsDir()
	if err != nil {
		return Result{}, &Error{Stage: StagePrepare, Err: err}
	}
	downloadsDir, err := s.resolver.DownloadsDir()
	if err != nil {
		return Result{}, &Error{Stage: StagePrepare, Err: err}
	}
	for _, dir := range []string{versionsDir, downloadsDir} {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return Result{}, &Error{Stage: StagePrepare, Err: fmt.Errorf("create directory %q: %w", dir, err)}
		}
	}
	// Drop recognisable leftovers from previously crashed installs.
	// Interrupted commit backups are preserved under recovery names.
	orphanWarnings := cleanOrphans(ctx, versionsDir, downloadsDir)

	stagingDir := filepath.Join(versionsDir, stagingPrefix+uniqueID())
	if err := os.Mkdir(stagingDir, dirMode); err != nil {
		return Result{}, &Error{Stage: StagePrepare, Err: fmt.Errorf("create staging directory: %w", err)}
	}
	partPath := filepath.Join(downloadsDir, partPrefix+uniqueID()+partSuffix)

	res, err := s.install(ctx, req, versionsDir, stagingDir, partPath)
	if err != nil {
		joinFailureCleanup(partPath, stagingDir, &err)
		return res, err
	}
	res.Warnings = append(orphanWarnings, res.Warnings...)
	return res, nil
}

// install runs the post-lock pipeline: download, integrity, extract,
// verify, commit. On success it also performs the non-fatal
// post-commit cleanup.
func (s *Service) install(ctx context.Context, req Request, versionsDir, stagingDir, partPath string) (Result, error) {
	var warnings []Warning

	_, digest, err := s.download(ctx, req, partPath)
	if err != nil {
		return Result{}, &Error{Stage: StageDownload, Err: err}
	}
	if req.SHA256 == "" {
		warnings = append(warnings, Warning{Kind: WarningIntegrityUnavailable})
	} else {
		expected, err := hex.DecodeString(req.SHA256)
		if err != nil {
			return Result{}, &Error{Stage: StageIntegrity, Err: fmt.Errorf("decode archive checksum: %w", err)}
		}
		if subtle.ConstantTimeCompare(digest, expected) != 1 {
			return Result{}, &Error{Stage: StageIntegrity, Err: errors.New("archive checksum mismatch")}
		}
	}

	if err := s.extract(ctx, partPath, stagingDir); err != nil {
		return Result{}, &Error{Stage: StageExtract, Err: fmt.Errorf("extract archive: %w", err)}
	}
	binaryPath := filepath.Join(stagingDir, "go", "bin", binaryName())
	if _, err := os.Stat(binaryPath); err != nil {
		return Result{}, &Error{Stage: StageVerify, Err: fmt.Errorf("extracted binary missing: %w", err)}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, &Error{Stage: StageVerify, Err: err}
	}
	out, err := s.verify(ctx, binaryPath)
	if err != nil {
		return Result{}, &Error{Stage: StageVerify, Err: fmt.Errorf("run go version: %w", err)}
	}
	if err := verifyVersionOutput(out, req.Version); err != nil {
		return Result{}, &Error{Stage: StageVerify, Err: err}
	}

	finalDir := filepath.Join(versionsDir, "go"+req.Version)
	backupDir, commitErr := s.commit(stagingDir, finalDir)
	if commitErr != nil {
		var stageErr *Error
		if errors.As(commitErr, &stageErr) {
			// Already a fully formed stage error (e.g. a preserved
			// recovery backup); surface it unchanged.
			return Result{}, stageErr
		}
		return Result{}, &Error{Stage: StageCommit, Err: commitErr}
	}

	// Post-commit cleanup is best-effort: failures are surfaced as
	// warnings rather than failing an otherwise successful install.
	var cleanupErrs []error
	for _, p := range []string{backupDir, stagingDir, partPath} {
		if p == "" {
			continue
		}
		if rmErr := s.cleanup(p); rmErr != nil {
			cleanupErrs = append(cleanupErrs, rmErr)
		}
	}
	if len(cleanupErrs) > 0 {
		warnings = append(warnings, Warning{Kind: WarningCleanup, Err: errors.Join(cleanupErrs...)})
	}

	return Result{Version: req.Version, Path: finalDir, Warnings: warnings}, nil
}

// download streams the archive into partPath exactly once. It returns
// the computed SHA-256 digest of the bytes written so callers can
// verify integrity without re-reading the file.
func (s *Service) download(ctx context.Context, req Request, partPath string) (int64, []byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := s.doer.Do(httpReq)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, nil, fmt.Errorf("download failed: HTTP %s", resp.Status)
	}

	partFile, err := os.OpenFile(partPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return 0, nil, fmt.Errorf("create download file: %w", err)
	}
	hasher := sha256.New()
	// Hard cap: read at most one byte beyond the maximum so an
	// overshoot is detectable without an unbounded copy.
	limited := io.LimitReader(&ctxReader{ctx: ctx, r: resp.Body}, MaxDownloadSize+1)
	written, copyErr := io.Copy(io.MultiWriter(partFile, hasher), limited)
	closeErr := partFile.Close()
	if copyErr != nil {
		return 0, nil, copyErr
	}
	if closeErr != nil {
		return 0, nil, fmt.Errorf("close download file: %w", closeErr)
	}
	if written > MaxDownloadSize {
		return 0, nil, fmt.Errorf("download exceeds maximum compressed size %d bytes", MaxDownloadSize)
	}
	if written == 0 {
		return 0, nil, errors.New("downloaded archive is empty")
	}
	if req.Size > 0 && written != req.Size {
		return 0, nil, fmt.Errorf("download size mismatch: got %d bytes, want %d", written, req.Size)
	}
	return written, hasher.Sum(nil), nil
}

// commit performs the transactional swap of staging/go -> finalDir.
//
// If finalDir already exists it is first renamed to a unique backup. On
// commit failure the backup is restored; if restoration also fails the
// backup is preserved under a .recovery-* path and the returned *Error
// carries RecoveryPath.
func (s *Service) commit(stagingDir, finalDir string) (backupDir string, err error) {
	goDir := filepath.Join(stagingDir, "go")
	if _, statErr := os.Stat(finalDir); statErr == nil {
		backupDir = filepath.Join(filepath.Dir(finalDir), backupPrefix+uniqueID())
		if mvErr := s.rename(finalDir, backupDir); mvErr != nil {
			return "", fmt.Errorf("back up existing install: %w", mvErr)
		}
	}
	if mvErr := s.rename(goDir, finalDir); mvErr != nil {
		if backupDir != "" {
			if rbErr := s.rename(backupDir, finalDir); rbErr != nil {
				return s.preserveBackup(backupDir, mvErr, rbErr)
			}
		}
		// Rollback succeeded (or there was nothing to roll back): the
		// backup no longer occupies backupDir, so drop the path.
		return "", mvErr
	}
	return backupDir, nil
}

// preserveBackup moves a rollback-failed backup to a distinct
// .recovery-* path and returns a StageCommit *Error carrying the
// recovery location. The rename to the recovery path uses os.Rename
// directly (not the test hook) so disaster recovery does not depend on
// the same hook that just failed twice.
func (s *Service) preserveBackup(backupDir string, commitErr, rollbackErr error) (string, error) {
	recoveryPath := filepath.Join(filepath.Dir(backupDir), recoveryPrefix+uniqueID())
	preserved := recoveryPath
	if mvErr := os.Rename(backupDir, recoveryPath); mvErr != nil {
		// Could not move: leave the backup where it is so the user can
		// recover it manually.
		preserved = backupDir
	}
	return "", &Error{
		Stage: StageCommit,
		Err: fmt.Errorf("commit failed: %w; rollback failed: %v",
			commitErr, rollbackErr),
		RecoveryPath: preserved,
	}
}

// verifyVersionOutput ensures the toolchain reports exactly the
// expected version and production platform.
func verifyVersionOutput(out []byte, version string) error {
	fields := strings.Fields(string(out))
	want := []string{
		"go",
		"version",
		"go" + version,
		runtime.GOOS + "/" + runtime.GOARCH,
	}
	if len(fields) != 4 ||
		fields[0] != want[0] ||
		fields[1] != want[1] ||
		fields[2] != want[2] ||
		fields[3] != want[3] {
		return fmt.Errorf("unexpected toolchain version output %q", strings.TrimSpace(string(out)))
	}
	return nil
}

// defaultCommandRunner runs the extracted go binary with the
// production platform's binary name.
func defaultCommandRunner(ctx context.Context, binary string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, "version")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	return cmd.CombinedOutput()
}

// binaryName returns the production go binary name for the host.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "go.exe"
	}
	return "go"
}

// newProductionHTTPClient returns a client restricted to HTTPS and the
// official Go download hosts for every redirect hop.
func newProductionHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-https scheme: %q", req.URL.Scheme)
			}
			host := req.URL.Hostname()
			if !strings.EqualFold(host, "go.dev") && !strings.EqualFold(host, "dl.google.com") {
				return fmt.Errorf("redirect to disallowed host: %q", host)
			}
			return nil
		},
	}
}

// validateRequest checks every invariant that can be verified without
// side effects.
func validateRequest(req Request) error {
	if req.Version == "" {
		return errors.New("version is required")
	}
	if !versionRe.MatchString(req.Version) {
		return fmt.Errorf("malformed version %q", req.Version)
	}
	expectedExt := "tar.gz"
	if runtime.GOOS == "windows" {
		expectedExt = "zip"
	}
	expectedFilename := "go" + req.Version + "." + runtime.GOOS + "-" + runtime.GOARCH + "." + expectedExt
	if req.Filename == "" {
		return errors.New("filename is required")
	}
	if req.Filename != expectedFilename {
		return fmt.Errorf("filename %q does not match expected %q for this platform", req.Filename, expectedFilename)
	}
	if req.Size < 0 {
		return fmt.Errorf("invalid size %d", req.Size)
	}
	if req.Size > MaxDownloadSize {
		return fmt.Errorf("declared size %d exceeds maximum %d", req.Size, MaxDownloadSize)
	}
	if req.SHA256 != "" {
		if len(req.SHA256) != sha256.Size*2 {
			return fmt.Errorf("invalid sha256 length %d", len(req.SHA256))
		}
		if _, err := hex.DecodeString(req.SHA256); err != nil {
			return fmt.Errorf("invalid sha256 hex: %w", err)
		}
	}
	if err := validateURL(req.URL, req.Filename); err != nil {
		return err
	}
	return nil
}

// validateURL enforces the official download URL shape.
func validateURL(raw, filename string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("url scheme must be https, got %q", u.Scheme)
	}
	if !strings.EqualFold(u.Hostname(), "go.dev") {
		return fmt.Errorf("url host must be go.dev, got %q", u.Hostname())
	}
	if !strings.HasPrefix(u.Path, "/dl/") {
		return fmt.Errorf("url path must be under /dl/, got %q", u.Path)
	}
	base := u.Path
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if base != filename {
		return fmt.Errorf("url basename %q does not match filename %q", base, filename)
	}
	return nil
}

// cleanOrphans removes recognisable in-progress artefacts left by
// previously crashed installs and preserves interrupted commit backups
// under recovery names. Recovery backups are never removed.
func cleanOrphans(ctx context.Context, versionsDir, downloadsDir string) []Warning {
	var warnings []Warning
	if err := ctx.Err(); err != nil {
		return warnings
	}
	if entries, err := os.ReadDir(versionsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			full := filepath.Join(versionsDir, name)
			if !paths.IsDirectChild(versionsDir, full) {
				continue
			}
			switch {
			case strings.HasPrefix(name, stagingPrefix):
				if err := os.RemoveAll(full); err != nil {
					warnings = append(warnings, Warning{
						Kind: WarningCleanup,
						Err:  fmt.Errorf("remove orphan staging %q: %w", full, err),
					})
				}
			case strings.HasPrefix(name, backupPrefix):
				recoveryPath := filepath.Join(versionsDir, recoveryPrefix+uniqueID())
				if err := os.Rename(full, recoveryPath); err != nil {
					warnings = append(warnings, Warning{
						Kind: WarningCleanup,
						Err:  fmt.Errorf("preserve interrupted installation backup %q: %w", full, err),
					})
					continue
				}
				warnings = append(warnings, Warning{
					Kind: WarningCleanup,
					Err:  fmt.Errorf("interrupted installation backup preserved at %s", recoveryPath),
				})
			}
		}
	}
	if entries, err := os.ReadDir(downloadsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, partPrefix) || !strings.HasSuffix(name, partSuffix) {
				continue
			}
			full := filepath.Join(downloadsDir, name)
			if !paths.IsDirectChild(downloadsDir, full) {
				continue
			}
			if err := os.RemoveAll(full); err != nil {
				warnings = append(warnings, Warning{
					Kind: WarningCleanup,
					Err:  fmt.Errorf("remove orphan download %q: %w", full, err),
				})
			}
		}
	}
	return warnings
}

// joinFailureCleanup removes the current download and staging
// artefacts after a failed install, joining any removal errors onto
// the original failure so they remain observable.
func joinFailureCleanup(partPath, stagingDir string, err *error) {
	var errs []error
	if *err != nil {
		errs = append(errs, *err)
	}
	if partPath != "" {
		if rmErr := os.RemoveAll(partPath); rmErr != nil {
			errs = append(errs, fmt.Errorf("remove download: %w", rmErr))
		}
	}
	if stagingDir != "" {
		if rmErr := os.RemoveAll(stagingDir); rmErr != nil {
			errs = append(errs, fmt.Errorf("remove staging: %w", rmErr))
		}
	}
	switch len(errs) {
	case 0:
	case 1:
		*err = errs[0]
	default:
		*err = errors.Join(errs...)
	}
}

// uniqueID returns a short hex identifier suitable for disambiguating
// staging, backup and recovery paths.
func uniqueID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// ctxReader aborts reads as soon as the context is cancelled, so long
// downloads do not outlive a cancelled install.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
