package paths

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func newResolver(t *testing.T, home, config string) *Resolver {
	t.Helper()
	return &Resolver{
		HomeDir: func() (string, error) {
			if home == "" {
				return "", errors.New("no home")
			}
			return home, nil
		},
		ConfigDir: func() (string, error) {
			if config == "" && home == "" {
				return "", errors.New("no config")
			}
			return config, nil
		},
	}
}

func TestResolverLayoutFromHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	r := newResolver(t, home, filepath.Join(t.TempDir(), "config"))

	cases := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"RootDir", r.RootDir, filepath.Join(home, ".govm")},
		{"VersionsDir", r.VersionsDir, filepath.Join(home, ".govm", "versions")},
		{"DownloadsDir", r.DownloadsDir, filepath.Join(home, ".govm", "downloads")},
		{"ShimDir", r.ShimDir, filepath.Join(home, ".govm", "shim")},
		{"ActiveVersionFile", r.ActiveVersionFile, filepath.Join(home, ".govm", "active_version")},
		{"SettingsFile", r.SettingsFile, filepath.Join(home, ".govm", "settings.json")},
		{"DepsBackupDir", r.DepsBackupDir, filepath.Join(home, ".govm", "deps_backup")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.got()
			if err != nil {
				t.Fatalf("%s() error = %v", tc.name, err)
			}
			if got != tc.want {
				t.Fatalf("%s() = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestLegacySettingsFilePaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	config := filepath.Join(t.TempDir(), "config")
	r := newResolver(t, home, config)

	got, ok, err := r.LegacySettingsFile()
	if err != nil {
		t.Fatalf("LegacySettingsFile() error = %v", err)
	}
	if !ok {
		t.Fatal("expected ok = true when config dir resolves")
	}
	want := filepath.Join(config, "govm", "settings.json")
	if got != want {
		t.Fatalf("LegacySettingsFile() = %q, want %q", got, want)
	}
}

func TestLegacySettingsFileEmptyConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	r := &Resolver{
		HomeDir: func() (string, error) { return home, nil },
		ConfigDir: func() (string, error) {
			return "", nil
		},
	}

	got, ok, err := r.LegacySettingsFile()
	if err != nil {
		t.Fatalf("LegacySettingsFile() error = %v", err)
	}
	if ok {
		t.Fatalf("expected ok = false, got true with path %q", got)
	}
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

func TestHomeDirErrorSurfaced(t *testing.T) {
	r := &Resolver{
		HomeDir:   func() (string, error) { return "", errors.New("nope") },
		ConfigDir: func() (string, error) { return "", nil },
	}

	if _, err := r.RootDir(); err == nil {
		t.Fatal("expected error from RootDir when HomeDir fails")
	}
	if _, err := r.SettingsFile(); err == nil {
		t.Fatal("expected error from SettingsFile when HomeDir fails")
	}
}

func TestEmptyHomePathTreatedAsError(t *testing.T) {
	r := &Resolver{
		HomeDir:   func() (string, error) { return "", nil },
		ConfigDir: func() (string, error) { return "", nil },
	}
	if _, err := r.RootDir(); err == nil {
		t.Fatal("expected error from RootDir when HomeDir returns empty")
	}
}

func TestNewResolverUsesStdlib(t *testing.T) {
	r := New()
	if r.HomeDir == nil || r.ConfigDir == nil {
		t.Fatal("New() should populate HomeDir and ConfigDir defaults")
	}

	home, err := r.HomeDir()
	if err != nil {
		t.Skipf("home directory unavailable in this environment: %v", err)
	}
	got, err := r.ShimDir()
	if err != nil {
		t.Fatalf("ShimDir() error = %v", err)
	}
	want := filepath.Join(home, ".govm", "shim")
	if runtime.GOOS == "windows" {
		_ = want // layout is the same; comparison is platform-safe via filepath.Join
	}
	if got != want {
		t.Fatalf("ShimDir() = %q, want %q", got, want)
	}
}

func TestIsDirectChild(t *testing.T) {
	root := filepath.Join(t.TempDir(), "versions")

	cases := []struct {
		name      string
		candidate string
		want      bool
	}{
		{"direct child", filepath.Join(root, "go1.24.0"), true},
		{"cleaned direct child", filepath.Join(root, "go1.24.0", "..", "go1.24.1"), true},
		{"root", root, false},
		{"traversal", filepath.Join(root, "..", "outside"), false},
		{"foreign absolute", filepath.Join(t.TempDir(), "go1.24.0"), false},
		{"nested child", filepath.Join(root, "go1.24.0", "bin"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDirectChild(root, tc.candidate); got != tc.want {
				t.Fatalf("IsDirectChild(%q, %q) = %t, want %t", root, tc.candidate, got, tc.want)
			}
		})
	}
}
