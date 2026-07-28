package install

import (
	"archive/tar"
	"context"
	"fmt"
	"runtime"
	"testing"
)

// buildManyFileTarGz produces a valid toolchain-shaped archive holding
// fileCount small regular files, so per-entry costs dominate.
func buildManyFileTarGz(t *testing.T, fileCount int) string {
	t.Helper()
	entries := []tEntry{
		{name: "go/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/bin/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/bin/" + binaryName(), typeflag: tar.TypeReg, mode: 0o755, content: []byte("bin")},
	}
	for i := range fileCount {
		entries = append(entries, tEntry{
			name:     fmt.Sprintf("go/pkg/f%05d.a", i),
			typeflag: tar.TypeReg,
			mode:     0o644,
			content:  []byte("x"),
		})
	}
	return buildTarGz(t, entries)
}

// extractAllocPerFile reports the bytes allocated per extracted file
// for a single extraction run.
func extractAllocPerFile(t *testing.T, fileCount int) uint64 {
	t.Helper()
	archive := buildManyFileTarGz(t, fileCount)
	dest := t.TempDir()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if err := extractArchive(context.Background(), archive, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	runtime.ReadMemStats(&after)

	return (after.TotalAlloc - before.TotalAlloc) / uint64(fileCount)
}

// TestExtractArchive_CopyBufferIsNotPerFile pins the copy buffer to the
// extraction run rather than to each entry.
//
// A Go toolchain archive holds roughly 14,000 files. Allocating a fresh
// 32 KiB copy buffer per entry churned ~450 MB of garbage per install,
// which grew the heap arenas and — because darwin releases pages with
// MADV_FREE — left the process resident size visibly higher after every
// installation.
//
// The threshold sits well below the 32 KiB buffer and well above the
// genuine per-entry cost (tar header, path bookkeeping, file handle).
func TestExtractArchive_CopyBufferIsNotPerFile(t *testing.T) {
	const fileCount = 2000
	const maxAllocPerFile = 8 * 1024

	perFile := extractAllocPerFile(t, fileCount)
	t.Logf("allocated %d bytes per extracted file", perFile)
	if perFile > maxAllocPerFile {
		t.Errorf("extraction allocates %d bytes per file, want at most %d "+
			"(a per-entry copy buffer would show up here)", perFile, maxAllocPerFile)
	}
}
