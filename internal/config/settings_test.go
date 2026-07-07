package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		t.Fatalf("Load() error = %v, want nil", err)
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
