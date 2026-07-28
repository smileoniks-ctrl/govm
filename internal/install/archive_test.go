package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tEntry describes a tar entry to write in buildTarGz.
type tEntry struct {
	name     string
	typeflag byte
	mode     int64
	content  []byte
	link     string
	devmaj   int64
	devmin   int64
}

func buildTarGz(t *testing.T, entries []tEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Typeflag: e.typeflag,
			Linkname: e.link,
			Size:     int64(len(e.content)),
			Format:   tar.FormatGNU,
			Devmajor: e.devmaj,
			Devminor: e.devmin,
		}
		if e.typeflag == tar.TypeDir || e.typeflag == tar.TypeSymlink ||
			e.typeflag == tar.TypeLink || e.typeflag == tar.TypeFifo ||
			e.typeflag == tar.TypeChar || e.typeflag == tar.TypeBlock {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.name, err)
		}
		if len(e.content) > 0 {
			if _, err := tw.Write(e.content); err != nil {
				t.Fatalf("write tar content %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return path
}

// zEntry describes a zip entry to write in buildZip.
type zEntry struct {
	name      string
	mode      os.FileMode
	isDir     bool
	isSymlink bool
	content   []byte
	target    string
}

func buildZip(t *testing.T, entries []zEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store}
		mode := e.mode
		if e.isDir {
			mode |= os.ModeDir
		}
		if e.isSymlink {
			mode |= os.ModeSymlink
		}
		h.SetMode(mode)
		if e.isDir {
			if !strings.HasSuffix(h.Name, "/") {
				h.Name += "/"
			}
			if _, err := w.CreateHeader(h); err != nil {
				t.Fatalf("create zip dir header %q: %v", e.name, err)
			}
			continue
		}
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatalf("create zip header %q: %v", e.name, err)
		}
		payload := e.content
		if e.isSymlink {
			payload = []byte(e.target)
		}
		if _, err := fw.Write(payload); err != nil {
			t.Fatalf("write zip content %q: %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return path
}

func TestExtractArchive_TarGzValid(t *testing.T) {
	archive := buildTarGz(t, []tEntry{
		{name: "go", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/bin", typeflag: tar.TypeDir, mode: 0o750},
		{name: "go/bin/go", typeflag: tar.TypeReg, mode: 0o755, content: []byte("binary")},
		{name: "go/README", typeflag: tar.TypeReg, mode: 0o644, content: []byte("docs")},
	})
	dest := t.TempDir()
	if err := extractArchive(context.Background(), archive, dest); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}

	st, err := os.Stat(filepath.Join(dest, "go"))
	if err != nil {
		t.Fatalf("stat go: %v", err)
	}
	if !st.IsDir() || st.Mode().Perm() != 0o755 {
		t.Fatalf("go dir mode = %v, want dir 0o755", st.Mode())
	}
	st, err = os.Stat(filepath.Join(dest, "go", "bin"))
	if err != nil {
		t.Fatalf("stat go/bin: %v", err)
	}
	if !st.IsDir() || st.Mode().Perm() != 0o750 {
		t.Fatalf("go/bin mode = %v, want dir 0o750", st.Mode())
	}
	st, err = os.Stat(filepath.Join(dest, "go", "bin", "go"))
	if err != nil {
		t.Fatalf("stat go/bin/go: %v", err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("go/bin/go mode = %v, want 0o755", st.Mode())
	}
	got, err := os.ReadFile(filepath.Join(dest, "go", "bin", "go"))
	if err != nil || string(got) != "binary" {
		t.Fatalf("go/bin/go content = %q err=%v, want \"binary\"", string(got), err)
	}
	st, err = os.Stat(filepath.Join(dest, "go", "README"))
	if err != nil {
		t.Fatalf("stat go/README: %v", err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("go/README mode = %v, want 0o644", st.Mode())
	}
}

func TestExtractArchive_ZipValid(t *testing.T) {
	archive := buildZip(t, []zEntry{
		{name: "go/", mode: 0o755, isDir: true},
		{name: "go/bin/", mode: 0o750, isDir: true},
		{name: "go/bin/go", mode: 0o755, content: []byte("binary")},
		{name: "go/README", mode: 0o644, content: []byte("docs")},
	})
	dest := t.TempDir()
	if err := extractArchive(context.Background(), archive, dest); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}

	st, err := os.Stat(filepath.Join(dest, "go"))
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0o755 {
		t.Fatalf("go dir: st=%v err=%v", st, err)
	}
	st, err = os.Stat(filepath.Join(dest, "go", "bin"))
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0o750 {
		t.Fatalf("go/bin dir: st=%v err=%v", st, err)
	}
	st, err = os.Stat(filepath.Join(dest, "go", "bin", "go"))
	if err != nil || st.Mode().Perm() != 0o755 {
		t.Fatalf("go/bin/go: st=%v err=%v", st, err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "go", "bin", "go"))
	if err != nil || string(got) != "binary" {
		t.Fatalf("go/bin/go content = %q err=%v", string(got), err)
	}
}

func TestExtractArchive_DetectsFormatFromPartFile(t *testing.T) {
	tests := []struct {
		name    string
		archive func(*testing.T) string
	}{
		{
			name: "tar.gz",
			archive: func(t *testing.T) string {
				return buildTarGz(t, []tEntry{
					{name: "go", typeflag: tar.TypeDir, mode: 0o755},
					{name: "go/bin", typeflag: tar.TypeDir, mode: 0o755},
					{name: "go/bin/go", typeflag: tar.TypeReg, mode: 0o755, content: []byte("binary")},
				})
			},
		},
		{
			name: "zip",
			archive: func(t *testing.T) string {
				return buildZip(t, []zEntry{
					{name: "go/", mode: 0o755, isDir: true},
					{name: "go/bin/", mode: 0o755, isDir: true},
					{name: "go/bin/go", mode: 0o755, content: []byte("binary")},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.archive(t))
			if err != nil {
				t.Fatalf("read archive: %v", err)
			}
			partPath := filepath.Join(t.TempDir(), ".govm-install-test.part")
			if err := os.WriteFile(partPath, data, 0o600); err != nil {
				t.Fatalf("write part archive: %v", err)
			}
			dest := t.TempDir()
			if err := extractArchive(context.Background(), partPath, dest); err != nil {
				t.Fatalf("extract part archive: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dest, "go", "bin", "go")); err != nil {
				t.Fatalf("extracted binary missing: %v", err)
			}
		})
	}
}

func TestValidatePath_RejectsUnsafe(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		dir  bool
	}{
		{"empty", "", false},
		{"nul byte", "go" + string(rune(0)) + "evil", false},
		{"backslash", "go\\evil", false},
		{"absolute", "/etc/passwd", false},
		{"absolute dir", "/go", true},
		{"dotdot top", "../go", true},
		{"parent traversal", "go/../../etc/passwd", false},
		{"dotdot middle", "go/sub/../../etc/x", false},
		{"outside tree", "other/x", false},
		{"top-level file", "go", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validatePath(tc.raw, tc.dir, defaultLimits); err == nil {
				t.Fatalf("validatePath(%q) expected error, got nil", tc.raw)
			}
		})
	}
}

func TestValidatePath_TooDeepAndLong(t *testing.T) {
	deep := "go"
	for i := 0; i < 100; i++ {
		deep += "/d"
	}
	deep += "/file.txt"
	if _, err := validatePath(deep, false, defaultLimits); err == nil {
		t.Fatalf("expected too-deep rejection")
	}
	long := "go/" + strings.Repeat("a", MaxArchivePath+1)
	if _, err := validatePath(long, false, defaultLimits); err == nil {
		t.Fatalf("expected too-long rejection")
	}
}

func TestValidatePath_AcceptsValid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		dir  bool
		want string
	}{
		{"top dir", "go", true, "go"},
		{"top dir trailing slash", "go/", true, "go"},
		{"nested file", "go/bin/go", false, "go/bin/go"},
		{"cleaned dots", "go/./bin/x", false, "go/bin/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validatePath(tc.raw, tc.dir, defaultLimits)
			if err != nil {
				t.Fatalf("validatePath(%q) unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("validatePath(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestExtractArchive_PathTraversal(t *testing.T) {
	deep := "go"
	for i := 0; i < 100; i++ {
		deep += "/d"
	}
	deep += "/file.txt"
	longName := "go/" + strings.Repeat("a", MaxArchivePath+1)

	cases := []struct {
		name    string
		entries []tEntry
	}{
		{"parent traversal", []tEntry{{name: "go/../../etc/passwd", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
		{"dotdot top", []tEntry{{name: "../go", typeflag: tar.TypeDir, mode: 0o755}}},
		{"absolute path", []tEntry{{name: "/etc/passwd", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
		{"absolute dir", []tEntry{{name: "/go", typeflag: tar.TypeDir, mode: 0o755}}},
		{"dotdot in middle", []tEntry{{name: "go/sub/../../etc/x", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
		{"backslash path", []tEntry{{name: "go\\evil", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
		{"too deep", []tEntry{{name: deep, typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
		{"too long", []tEntry{{name: longName, typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, tc.entries)
			dest := t.TempDir()
			err := extractArchive(context.Background(), archive, dest)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			// Ensure nothing escaped the destination tree.
			if _, statErr := os.Stat(filepath.Join(dest, "etc")); statErr == nil {
				t.Fatalf("escape created entry outside destination")
			}
		})
	}
}

func TestExtractArchive_RejectsSpecialEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []tEntry
	}{
		{"symlink", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/link", typeflag: tar.TypeSymlink, mode: 0o777, link: "../../etc/passwd"},
		}},
		{"hardlink", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("a")},
			{name: "go/b", typeflag: tar.TypeLink, mode: 0o644, link: "go/a"},
		}},
		{"char device", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/dev", typeflag: tar.TypeChar, mode: 0o644, devmaj: 1, devmin: 5},
		}},
		{"block device", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/blk", typeflag: tar.TypeBlock, mode: 0o644, devmaj: 1, devmin: 5},
		}},
		{"fifo", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/pipe", typeflag: tar.TypeFifo, mode: 0o644},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, tc.entries)
			dest := t.TempDir()
			if err := extractArchive(context.Background(), archive, dest); err == nil {
				t.Fatalf("expected rejection of %s, got nil", tc.name)
			}
		})
	}
}

func TestExtractArchive_ZipRejectsSymlink(t *testing.T) {
	archive := buildZip(t, []zEntry{
		{name: "go/", mode: 0o755, isDir: true},
		{name: "go/link", mode: 0o777, isSymlink: true, target: "../../etc/passwd"},
	})
	dest := t.TempDir()
	if err := extractArchive(context.Background(), archive, dest); err == nil {
		t.Fatalf("expected rejection of zip symlink, got nil")
	}
}

func TestExtractArchive_DuplicatePath(t *testing.T) {
	archive := buildTarGz(t, []tEntry{
		{name: "go", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("first")},
		{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("second")},
	})
	if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
		t.Fatalf("expected duplicate path rejection, got nil")
	}
}

func TestExtractArchive_DuplicateDirectory(t *testing.T) {
	archive := buildTarGz(t, []tEntry{
		{name: "go", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/d", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/d", typeflag: tar.TypeDir, mode: 0o755},
	})
	if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
		t.Fatalf("expected repeated directory rejection, got nil")
	}
}

func TestExtractArchive_DirFileCollision(t *testing.T) {
	cases := []struct {
		name    string
		entries []tEntry
	}{
		{"dir then file", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")},
		}},
		{"file then dir", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")},
			{name: "go/a", typeflag: tar.TypeDir, mode: 0o755},
		}},
		{"parent under file", []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")},
			{name: "go/a/b", typeflag: tar.TypeReg, mode: 0o644, content: []byte("y")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, tc.entries)
			if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
				t.Fatalf("expected collision rejection, got nil")
			}
		})
	}
}

func TestExtractArchive_MultipleTopLevelRoots(t *testing.T) {
	archive := buildTarGz(t, []tEntry{
		{name: "go", typeflag: tar.TypeDir, mode: 0o755},
		{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")},
		{name: "other", typeflag: tar.TypeDir, mode: 0o755},
	})
	if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
		t.Fatalf("expected rejection of additional top-level root, got nil")
	}
}

func TestExtractArchive_TopLevelMustBeDir(t *testing.T) {
	archive := buildTarGz(t, []tEntry{
		{name: "go", typeflag: tar.TypeReg, mode: 0o644, content: []byte("not a dir")},
	})
	if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
		t.Fatalf("expected rejection of file at top level, got nil")
	}
}

func TestExtractArchive_MissingGoDir(t *testing.T) {
	archive := buildTarGz(t, nil)
	if err := extractArchive(context.Background(), archive, t.TempDir()); err == nil {
		t.Fatalf("expected missing-go rejection for empty archive, got nil")
	}
}

func TestExtractArchive_RequiresExplicitGoDir(t *testing.T) {
	tests := []struct {
		name    string
		archive func(*testing.T) string
	}{
		{
			name: "tar.gz",
			archive: func(t *testing.T) string {
				return buildTarGz(t, []tEntry{
					{name: "go/bin/go", typeflag: tar.TypeReg, mode: 0o755, content: []byte("binary")},
				})
			},
		},
		{
			name: "zip",
			archive: func(t *testing.T) string {
				return buildZip(t, []zEntry{
					{name: "go/bin/go", mode: 0o755, content: []byte("binary")},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := extractArchive(context.Background(), tt.archive(t), t.TempDir()); err == nil {
				t.Fatal("expected rejection without explicit top-level go directory")
			}
		})
	}
}

func TestExtractArchive_Limits(t *testing.T) {
	t.Run("entry count", func(t *testing.T) {
		entries := []tEntry{{name: "go", typeflag: tar.TypeDir, mode: 0o755}}
		for i := 0; i < 4; i++ {
			entries = append(entries, tEntry{
				name:     "go/f" + string(rune('a'+i)),
				typeflag: tar.TypeReg,
				mode:     0o644,
				content:  []byte("x"),
			})
		}
		archive := buildTarGz(t, entries)
		lim := defaultLimits
		lim.maxFiles = 3
		err := extractArchiveWith(context.Background(), archive, t.TempDir(), lim)
		if err == nil || !strings.Contains(err.Error(), "entry count") {
			t.Fatalf("expected entry-count error, got %v", err)
		}
	})

	t.Run("single file size", func(t *testing.T) {
		entries := []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/big", typeflag: tar.TypeReg, mode: 0o644, content: bytes.Repeat([]byte("x"), 50)},
		}
		archive := buildTarGz(t, entries)
		lim := defaultLimits
		lim.maxFile = 10
		err := extractArchiveWith(context.Background(), archive, t.TempDir(), lim)
		if err == nil || !strings.Contains(err.Error(), "max file size") {
			t.Fatalf("expected max-file-size error, got %v", err)
		}
	})

	t.Run("total size", func(t *testing.T) {
		entries := []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: bytes.Repeat([]byte("a"), 20)},
			{name: "go/b", typeflag: tar.TypeReg, mode: 0o644, content: bytes.Repeat([]byte("b"), 20)},
		}
		archive := buildTarGz(t, entries)
		lim := defaultLimits
		lim.maxFile = 1000
		lim.maxSize = 30
		err := extractArchiveWith(context.Background(), archive, t.TempDir(), lim)
		if err == nil || !strings.Contains(err.Error(), "total extracted size") {
			t.Fatalf("expected total-size error, got %v", err)
		}
	})
}

func TestExtractArchive_ContextCancellation(t *testing.T) {
	t.Run("pre-cancelled between entries", func(t *testing.T) {
		archive := buildTarGz(t, []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("x")},
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := extractArchive(ctx, archive, t.TempDir())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("during copy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var total int64
		dst := &bytes.Buffer{}
		done := make(chan error, 1)
		go func() {
			done <- copyOut(ctx, dst, &foreverReader{ctx: ctx}, "blocked", &total, defaultLimits, make([]byte, copyBufferSize))
		}()
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context.Canceled during copy, got %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("copyOut did not return after cancellation")
		}
	})
}

type foreverReader struct{ ctx context.Context }

func (r *foreverReader) Read(p []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func TestExtractArchive_CorruptedArchives(t *testing.T) {
	t.Run("corrupted gzip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.tar.gz")
		if err := os.WriteFile(path, []byte("this is not gzip data at all"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := extractArchive(context.Background(), path, t.TempDir()); err == nil {
			t.Fatalf("expected gzip error, got nil")
		}
	})

	t.Run("corrupted tar body", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.tar.gz")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		gw := gzip.NewWriter(f)
		if _, err := gw.Write([]byte("not a valid tar stream content at all")); err != nil {
			t.Fatal(err)
		}
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if err := extractArchive(context.Background(), path, t.TempDir()); err == nil {
			t.Fatalf("expected tar error, got nil")
		}
	})

	t.Run("corrupted zip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.zip")
		if err := os.WriteFile(path, []byte("PK\x03\x04 this is definitely not a real zip file body"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := extractArchive(context.Background(), path, t.TempDir()); err == nil {
			t.Fatalf("expected zip error, got nil")
		}
	})
}

func TestExtractArchive_NoOverwritePreExisting(t *testing.T) {
	t.Run("pre-existing file", func(t *testing.T) {
		dest := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dest, "go"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "go", "a"), []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
		archive := buildTarGz(t, []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
			{name: "go/a", typeflag: tar.TypeReg, mode: 0o644, content: []byte("replacement")},
		})
		err := extractArchive(context.Background(), archive, dest)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("expected already-exists error, got %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "go", "a"))
		if err != nil || string(got) != "original" {
			t.Fatalf("pre-existing file overwritten: got %q err=%v", string(got), err)
		}
	})

	t.Run("pre-existing dir", func(t *testing.T) {
		dest := t.TempDir()
		if err := os.Mkdir(filepath.Join(dest, "go"), 0o755); err != nil {
			t.Fatal(err)
		}
		archive := buildTarGz(t, []tEntry{
			{name: "go", typeflag: tar.TypeDir, mode: 0o755},
		})
		err := extractArchive(context.Background(), archive, dest)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("expected already-exists error, got %v", err)
		}
	})
}

func TestExtractArchive_UnsupportedType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weird.rar")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(context.Background(), path, t.TempDir()); err == nil {
		t.Fatalf("expected unsupported-type error, got nil")
	}
}
