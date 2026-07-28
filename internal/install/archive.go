package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// topLevelDir is the only top-level entry permitted in a supported archive.
const topLevelDir = "go"

type entryKind int

const (
	kindFile entryKind = iota
	kindDir
)

// limits bundles the configurable safety ceilings used while extracting.
// extractArchive uses the package constants via defaultLimits; tests may pass a
// reduced set so size/count limits can be exercised without allocating GiBs.
type limits struct {
	maxFiles int
	maxSize  int64
	maxFile  int64
	maxDepth int
	maxPath  int
}

var defaultLimits = limits{
	maxFiles: MaxArchiveFiles,
	maxSize:  MaxExtractedSize,
	maxFile:  MaxFileSize,
	maxDepth: MaxArchiveDepth,
	maxPath:  MaxArchivePath,
}

// extractArchive unpacks a .tar.gz or .zip archive at archivePath into the
// destination staging directory using only the standard library. It enforces
// path, type, count, and size limits; on any failure it returns a wrapped error
// and leaves cleanup to the caller.
func extractArchive(ctx context.Context, archivePath, destination string) error {
	return extractArchiveWith(ctx, archivePath, destination, defaultLimits)
}

func extractArchiveWith(ctx context.Context, archivePath, destination string, lim limits) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	format, err := detectArchiveFormat(archivePath)
	if err != nil {
		return err
	}
	switch format {
	case "tar.gz":
		return extractTarGz(ctx, archivePath, destination, lim)
	case "zip":
		return extractZip(ctx, archivePath, destination, lim)
	default:
		return fmt.Errorf("extract: unsupported archive type %q", archivePath)
	}
}

func detectArchiveFormat(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("extract: open archive: %w", err)
	}
	defer f.Close()

	var signature [4]byte
	n, err := io.ReadFull(f, signature[:])
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("extract: read archive signature: %w", err)
	}
	if n >= 2 && signature[0] == 0x1f && signature[1] == 0x8b {
		return "tar.gz", nil
	}
	if n == len(signature) &&
		signature[0] == 'P' &&
		signature[1] == 'K' &&
		((signature[2] == 3 && signature[3] == 4) ||
			(signature[2] == 5 && signature[3] == 6) ||
			(signature[2] == 7 && signature[3] == 8)) {
		return "zip", nil
	}
	return "", fmt.Errorf("extract: unsupported archive type %q", archivePath)
}

// copyBufferSize is the streaming window used to move one entry's bytes
// to disk. One buffer serves the whole extraction: a Go toolchain holds
// roughly 14,000 files, so a per-entry buffer would churn hundreds of
// megabytes of garbage for every install.
const copyBufferSize = 32 * 1024

// guard tracks extraction state: the set of committed paths (for duplicate and
// collision detection), the set of filesystem entries created in this run, the
// running totals used to enforce size and count ceilings, and the copy buffer
// shared by every entry.
type guard struct {
	dest    string
	seen    map[string]entryKind
	made    map[string]bool
	buf     []byte
	total   int64
	entries int
	lim     limits
	topDir  bool
}

func newGuard(dest string, lim limits) *guard {
	return &guard{
		dest: dest,
		seen: make(map[string]entryKind),
		made: make(map[string]bool),
		buf:  make([]byte, copyBufferSize),
		lim:  lim,
	}
}

// validatePath inspects the raw archive path and returns its cleaned,
// slash-separated, within-tree form. It rejects empty, absolute, escaping,
// backslash, NUL-bearing, too-long, too-deep, and out-of-tree names.
func validatePath(raw string, isDir bool, lim limits) (string, error) {
	if len(raw) > lim.maxPath {
		return "", fmt.Errorf("entry path too long (%d bytes): %q", len(raw), raw)
	}
	if strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("entry path contains NUL byte: %q", raw)
	}
	if strings.Contains(raw, `\`) {
		return "", fmt.Errorf("entry path contains backslash: %q", raw)
	}
	name := strings.TrimRight(raw, "/")
	cleaned := path.Clean(name)
	if cleaned == "" || cleaned == "." || cleaned == ".." || path.IsAbs(cleaned) {
		return "", fmt.Errorf("unsafe entry path: %q", raw)
	}
	parts := strings.Split(cleaned, "/")
	if parts[0] != topLevelDir {
		return "", fmt.Errorf("entry %q is outside the %q/ tree", raw, topLevelDir)
	}
	if len(parts)-1 > lim.maxDepth {
		return "", fmt.Errorf("entry path too deep (%d): %q", len(parts)-1, raw)
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", fmt.Errorf("unsafe entry path component in %q", raw)
		}
	}
	if cleaned == topLevelDir && !isDir {
		return "", fmt.Errorf("top-level %q must be a directory", topLevelDir)
	}
	return cleaned, nil
}

// register records an entry together with its implicit ancestor directories,
// rejecting duplicate normalized paths and any file/dir nesting conflicts.
func (g *guard) register(cleaned string, k entryKind) error {
	parts := strings.Split(cleaned, "/")
	cur := ""
	for i, p := range parts {
		if i == 0 {
			cur = p
		} else {
			cur = cur + "/" + p
		}
		want := kindDir
		if i == len(parts)-1 {
			want = k
		}
		if got, ok := g.seen[cur]; ok {
			if i == len(parts)-1 {
				return fmt.Errorf("duplicate entry %q", cur)
			}
			if got != kindDir {
				return fmt.Errorf("entry %q nests beneath file %q", cleaned, cur)
			}
			continue
		}
		g.seen[cur] = want
	}
	return nil
}

// abs resolves a cleaned slash path to an absolute filesystem path beneath the
// destination, using filepath.Rel for separator-aware containment so that any
// escape is rejected regardless of platform.
func (g *guard) abs(cleaned string) (string, error) {
	full := filepath.Join(g.dest, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(g.dest, full)
	if err != nil {
		return "", fmt.Errorf("extract: cannot resolve %q: %w", cleaned, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("extract: entry %q escapes destination", cleaned)
	}
	return full, nil
}

func (g *guard) checkEntryCount() error {
	g.entries++
	if g.entries > g.lim.maxFiles {
		return fmt.Errorf("extract: archive exceeds max entry count (%d)", g.lim.maxFiles)
	}
	return nil
}

func (g *guard) createAncestors(cleaned string) error {
	parts := strings.Split(cleaned, "/")
	cur := ""
	for i := 0; i < len(parts)-1; i++ {
		if i == 0 {
			cur = parts[i]
		} else {
			cur = cur + "/" + parts[i]
		}
		if g.made[cur] {
			continue
		}
		full, err := g.abs(cur)
		if err != nil {
			return err
		}
		if err := os.Mkdir(full, 0o755); err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("extract: destination entry already exists: %q", cur)
			}
			return fmt.Errorf("extract: mkdir %q: %w", cur, err)
		}
		g.made[cur] = true
	}
	return nil
}

func (g *guard) makeDir(cleaned string, mode os.FileMode) error {
	if err := g.createAncestors(cleaned); err != nil {
		return err
	}
	full, err := g.abs(cleaned)
	if err != nil {
		return err
	}
	if g.made[cleaned] {
		// Created implicitly as an ancestor; align its mode to the archive entry.
		return os.Chmod(full, mode&0o777)
	}
	if err := os.Mkdir(full, mode&0o777); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("extract: destination entry already exists: %q", cleaned)
		}
		return fmt.Errorf("extract: mkdir %q: %w", cleaned, err)
	}
	g.made[cleaned] = true
	if err := os.Chmod(full, mode&0o777); err != nil {
		return fmt.Errorf("extract: chmod %q: %w", cleaned, err)
	}
	return nil
}

func (g *guard) makeFile(ctx context.Context, cleaned string, mode os.FileMode, src io.Reader) error {
	if err := g.createAncestors(cleaned); err != nil {
		return err
	}
	full, err := g.abs(cleaned)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode&0o777)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("extract: destination entry already exists: %q", cleaned)
		}
		return fmt.Errorf("extract: create %q: %w", cleaned, err)
	}
	copyErr := copyOut(ctx, f, src, cleaned, &g.total, g.lim, g.buf)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return fmt.Errorf("extract: close %q: %w", cleaned, closeErr)
	}
	if err := os.Chmod(full, mode&0o777); err != nil {
		return fmt.Errorf("extract: chmod %q: %w", cleaned, err)
	}
	g.made[cleaned] = true
	return nil
}

// copyOut streams src into dst while enforcing the per-file and total size
// limits against the actual bytes transferred (never trusting headers) and
// honoring context cancellation between and during reads. It borrows the
// guard's buffer, so extracting one more entry costs no extra memory.
func copyOut(ctx context.Context, dst io.Writer, src io.Reader, name string, total *int64, lim limits, buf []byte) error {
	cr := &ctxReader{ctx: ctx, r: src}
	var fileWritten int64
	for {
		n, rerr := cr.Read(buf)
		if n > 0 {
			fileWritten += int64(n)
			if fileWritten > lim.maxFile {
				return fmt.Errorf("extract: entry %q exceeds max file size (%d bytes)", name, lim.maxFile)
			}
			*total += int64(n)
			if *total < 0 || *total > lim.maxSize {
				return fmt.Errorf("extract: total extracted size exceeds limit (%d bytes)", lim.maxSize)
			}
			w, werr := dst.Write(buf[:n])
			if werr != nil {
				return fmt.Errorf("extract: write %q: %w", name, werr)
			}
			if w != n {
				return fmt.Errorf("extract: short write for %q", name)
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return nil
			}
			return fmt.Errorf("extract: read %q: %w", name, rerr)
		}
	}
}

func extractTarGz(ctx context.Context, archivePath, destination string, lim limits) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("extract: open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(&ctxReader{ctx: ctx, r: f})
	if err != nil {
		return fmt.Errorf("extract: read gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	g := newGuard(destination, lim)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract: read tar: %w", err)
		}
		if err := g.checkEntryCount(); err != nil {
			return err
		}
		if err := handleTarEntry(ctx, g, hdr, tr); err != nil {
			return err
		}
	}
	if !g.topDir {
		return fmt.Errorf("extract: archive missing top-level %q/ directory", topLevelDir)
	}
	return nil
}

func handleTarEntry(ctx context.Context, g *guard, hdr *tar.Header, tr *tar.Reader) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		cleaned, err := validatePath(hdr.Name, true, g.lim)
		if err != nil {
			return err
		}
		if err := g.register(cleaned, kindDir); err != nil {
			return err
		}
		if cleaned == topLevelDir {
			g.topDir = true
		}
		return g.makeDir(cleaned, os.FileMode(hdr.Mode))
	case tar.TypeReg, tar.TypeRegA:
		cleaned, err := validatePath(hdr.Name, false, g.lim)
		if err != nil {
			return err
		}
		if err := g.register(cleaned, kindFile); err != nil {
			return err
		}
		return g.makeFile(ctx, cleaned, os.FileMode(hdr.Mode), tr)
	default:
		return fmt.Errorf("extract: unsupported tar entry type %d for %q", hdr.Typeflag, hdr.Name)
	}
}

func extractZip(ctx context.Context, archivePath, destination string, lim limits) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("extract: open zip: %w", err)
	}
	defer zr.Close()
	if len(zr.File) > lim.maxFiles {
		return fmt.Errorf("extract: archive exceeds max entry count (%d)", lim.maxFiles)
	}
	g := newGuard(destination, lim)
	for _, zf := range zr.File {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		if err := g.checkEntryCount(); err != nil {
			return err
		}
		if err := handleZipEntry(ctx, g, zf); err != nil {
			return err
		}
	}
	if !g.topDir {
		return fmt.Errorf("extract: archive missing top-level %q/ directory", topLevelDir)
	}
	return nil
}

func handleZipEntry(ctx context.Context, g *guard, zf *zip.File) error {
	mode := zf.FileInfo().Mode()
	isDir := mode.IsDir()
	// Only directories and regular files are permitted.
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("extract: symlink entries are not allowed: %q", zf.Name)
	}
	if !isDir && mode&os.ModeType != 0 {
		return fmt.Errorf("extract: unsupported zip entry type %s for %q", mode.Type(), zf.Name)
	}
	cleaned, err := validatePath(zf.Name, isDir, g.lim)
	if err != nil {
		return err
	}
	if isDir {
		if err := g.register(cleaned, kindDir); err != nil {
			return err
		}
		if cleaned == topLevelDir {
			g.topDir = true
		}
		return g.makeDir(cleaned, mode.Perm())
	}
	if err := g.register(cleaned, kindFile); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("extract: open zip entry %q: %w", zf.Name, err)
	}
	copyErr := g.makeFile(ctx, cleaned, mode.Perm(), rc)
	closeErr := rc.Close()
	if copyErr != nil {
		if closeErr != nil {
			return errors.Join(copyErr, fmt.Errorf("extract: close zip entry %q: %w", zf.Name, closeErr))
		}
		return copyErr
	}
	if closeErr != nil {
		return fmt.Errorf("extract: close zip entry %q: %w", zf.Name, closeErr)
	}
	return nil
}
