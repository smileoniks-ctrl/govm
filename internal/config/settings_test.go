package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/paths"
)

func TestDefaultSettings(t *testing.T) {
	got := DefaultSettings()

	if got.DepsDisplay != DepsDisplayDirect {
		t.Fatalf("DepsDisplay = %q, want %q", got.DepsDisplay, DepsDisplayDirect)
	}
	if got.Theme != ThemeCurrent {
		t.Fatalf("Theme = %q, want %q", got.Theme, ThemeCurrent)
	}
}

func TestNormalizeUnknowns(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		want     Settings
	}{
		{
			name: "keeps known values",
			settings: Settings{
				DepsDisplay: DepsDisplayAll,
				Theme:       ThemeLight,
			},
			want: Settings{
				DepsDisplay: DepsDisplayAll,
				Theme:       ThemeLight,
			},
		},
		{
			name:     "defaults empty values",
			settings: Settings{},
			want:     DefaultSettings(),
		},
		{
			name: "defaults unknown values",
			settings: Settings{
				DepsDisplay: DepsDisplayMode("transitive"),
				Theme:       ThemeName("dark"),
			},
			want: DefaultSettings(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.settings)
			if got != tt.want {
				t.Fatalf("Normalize(%+v) = %+v, want %+v", tt.settings, got, tt.want)
			}
		})
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "settings.json")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("Load() = %+v, want %+v", got, DefaultSettings())
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	settings := Settings{
		DepsDisplay: DepsDisplayAll,
		Theme:       ThemeLight,
	}

	if err := Save(path, settings); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("file mode = %v, want %v", got, os.FileMode(0o644))
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got != settings {
		t.Fatalf("Load() = %+v, want %+v", got, settings)
	}
}

func TestSaveWritesNormalizedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := Settings{
		DepsDisplay: DepsDisplayMode("unknown"),
		Theme:       ThemeName("unknown"),
	}

	if err := Save(path, settings); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("Load() = %+v, want %+v", got, DefaultSettings())
	}
}

func TestSaveUsesTempFileAndLeavesNoTempFilesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := Save(path, Settings{DepsDisplay: DepsDisplayAll, Theme: ThemeLight}); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), tempFilePrefix) {
			t.Fatalf("Save() left temp file %q behind", entry.Name())
		}
	}
}

func TestSavePreservesExistingFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	oldData := []byte("{\"depsDisplay\":\"direct\",\"theme\":\"current\"}\n")
	if err := os.WriteFile(path, oldData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	renameErr := errors.New("rename failed")
	err := saveWithRename(path, Settings{DepsDisplay: DepsDisplayAll, Theme: ThemeLight}, func(_, _ string) error {
		return renameErr
	})
	if !errors.Is(err, renameErr) {
		t.Fatalf("Save() error = %v, want wrapped %v", err, renameErr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(oldData) {
		t.Fatalf("settings file = %q, want original %q", got, oldData)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), tempFilePrefix) {
			t.Fatalf("Save() left temp file %q behind", entry.Name())
		}
	}
}

// stubResolver returns a paths.Resolver with deterministic
// HomeDir and ConfigDir under fresh t.TempDir() directories so
// tests do not touch the real filesystem. The returned home and
// config paths are exposed so a test can pre-populate the
// directories or plant blocker files inside them.
func stubResolver(t *testing.T) (*paths.Resolver, string, string) {
	t.Helper()
	home := t.TempDir()
	config := t.TempDir()
	return &paths.Resolver{
		HomeDir:   func() (string, error) { return home, nil },
		ConfigDir: func() (string, error) { return config, nil },
	}, home, config
}

func TestDefaultPathUsesHomeDir(t *testing.T) {
	stubHome := t.TempDir()
	t.Setenv("HOME", stubHome)

	resolver := &paths.Resolver{
		HomeDir:   func() (string, error) { return stubHome, nil },
		ConfigDir: func() (string, error) { return "", nil },
	}
	want, err := resolver.SettingsFile()
	if err != nil {
		t.Fatalf("resolver.SettingsFile() error = %v", err)
	}

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q (from stub resolver)", got, want)
	}
	if !strings.HasSuffix(got, ".govm/settings.json") {
		t.Fatalf("DefaultPath() = %q, want suffix .govm/settings.json", got)
	}
	expectedRoot, err := resolver.RootDir()
	if err != nil {
		t.Fatalf("resolver.RootDir() error = %v", err)
	}
	if filepath.Dir(got) != expectedRoot {
		t.Fatalf("DefaultPath() parent dir = %q, want %q (from stub resolver HomeDir)", filepath.Dir(got), expectedRoot)
	}
}

func TestLoadWithMigrationNewFilePresent(t *testing.T) {
	r, _, _ := stubResolver(t)
	newPath, err := r.SettingsFile()
	if err != nil {
		t.Fatalf("SettingsFile() error = %v", err)
	}
	legacyPath, _, err := r.LegacySettingsFile()
	if err != nil {
		t.Fatalf("LegacySettingsFile() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	newData := []byte("{\"depsDisplay\":\"all\",\"theme\":\"light\"}\n")
	if err := os.WriteFile(newPath, newData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Also create a legacy file to verify it is not touched.
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacyData := []byte("{\"depsDisplay\":\"direct\",\"theme\":\"current\"}\n")
	if err := os.WriteFile(legacyPath, legacyData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	gotPath, gotSettings, migrated, err := loadWithMigrationFor(r)
	if err != nil {
		t.Fatalf("loadWithMigrationFor() error = %v", err)
	}
	if gotPath != newPath {
		t.Fatalf("path = %q, want %q", gotPath, newPath)
	}
	if migrated {
		t.Fatal("migrated = true, want false")
	}
	want := Settings{DepsDisplay: DepsDisplayAll, Theme: ThemeLight}
	if gotSettings != want {
		t.Fatalf("settings = %+v, want %+v", gotSettings, want)
	}

	// New file content is unchanged.
	newAfter, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("ReadFile(new) error = %v", err)
	}
	if string(newAfter) != string(newData) {
		t.Fatalf("new file = %q, want %q", newAfter, newData)
	}

	// Legacy file is still present and unchanged.
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("ReadFile(legacy) error = %v", err)
	}
	if string(legacyAfter) != string(legacyData) {
		t.Fatalf("legacy file = %q, want %q", legacyAfter, legacyData)
	}
}

func TestLoadWithMigrationMigratesLegacy(t *testing.T) {
	r, _, _ := stubResolver(t)
	newPath, err := r.SettingsFile()
	if err != nil {
		t.Fatalf("SettingsFile() error = %v", err)
	}
	legacyPath, _, err := r.LegacySettingsFile()
	if err != nil {
		t.Fatalf("LegacySettingsFile() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacyData := []byte("{\"depsDisplay\":\"all\",\"theme\":\"light\"}\n")
	if err := os.WriteFile(legacyPath, legacyData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	gotPath, gotSettings, migrated, err := loadWithMigrationFor(r)
	if err != nil {
		t.Fatalf("loadWithMigrationFor() error = %v", err)
	}
	if gotPath != newPath {
		t.Fatalf("path = %q, want %q", gotPath, newPath)
	}
	if !migrated {
		t.Fatal("migrated = false, want true")
	}
	want := Settings{DepsDisplay: DepsDisplayAll, Theme: ThemeLight}
	if gotSettings != want {
		t.Fatalf("settings = %+v, want %+v", gotSettings, want)
	}

	// New file must exist with migrated content.
	newData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("ReadFile(new) error = %v", err)
	}
	if !strings.Contains(string(newData), "\"all\"") {
		t.Fatalf("new file = %q, want to contain \"all\"", newData)
	}
	if !strings.Contains(string(newData), "\"light\"") {
		t.Fatalf("new file = %q, want to contain \"light\"", newData)
	}

	// Legacy file must be removed.
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy file still present: %v", err)
	}
}

func TestLoadWithMigrationSaveFails(t *testing.T) {
	r, stubHome, _ := stubResolver(t)
	newPath, err := r.SettingsFile()
	if err != nil {
		t.Fatalf("SettingsFile() error = %v", err)
	}
	legacyPath, _, err := r.LegacySettingsFile()
	if err != nil {
		t.Fatalf("LegacySettingsFile() error = %v", err)
	}

	// Plant a regular file at ~/.govm so MkdirAll fails when Save
	// tries to create the settings directory.
	govmDir := filepath.Join(stubHome, ".govm")
	if err := os.WriteFile(govmDir, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Setup a legacy file with real content.
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacyData := []byte("{\"depsDisplay\":\"all\",\"theme\":\"light\"}\n")
	if err := os.WriteFile(legacyPath, legacyData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	gotPath, gotSettings, migrated, err := loadWithMigrationFor(r)
	if err == nil {
		t.Fatal("loadWithMigrationFor() error = nil, want error")
	}
	if gotPath != "" {
		t.Fatalf("path = %q, want empty", gotPath)
	}
	if migrated {
		t.Fatal("migrated = true, want false")
	}
	if gotSettings != (Settings{}) {
		t.Fatalf("settings = %+v, want zero value", gotSettings)
	}

	// Legacy file must still be present because migration failed.
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("ReadFile(legacy) error = %v", err)
	}
	if string(legacyAfter) != string(legacyData) {
		t.Fatalf("legacy file = %q, want %q", legacyAfter, legacyData)
	}

	// The new file must not have been written. We only care that no
	// regular file exists at newPath; os.Stat may return ENOTDIR
	// because the parent is a regular file, which is also fine.
	if _, err := os.Stat(newPath); err == nil {
		t.Fatalf("new file present despite save failure")
	}
}

func TestLoadWithMigrationBothAbsent(t *testing.T) {
	r, _, _ := stubResolver(t)

	gotPath, gotSettings, migrated, err := loadWithMigrationFor(r)
	if err != nil {
		t.Fatalf("loadWithMigrationFor() error = %v", err)
	}
	if gotPath != "" {
		t.Fatalf("path = %q, want empty", gotPath)
	}
	if migrated {
		t.Fatal("migrated = true, want false")
	}
	if gotSettings != DefaultSettings() {
		t.Fatalf("settings = %+v, want %+v", gotSettings, DefaultSettings())
	}
}
