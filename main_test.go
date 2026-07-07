package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestPrintUsageShowsVersion(t *testing.T) {
	prev := utils.Version
	utils.Version = "v9.9.9-test"
	defer func() { utils.Version = prev }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = orig

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)

	if !strings.Contains(output, "v9.9.9-test") {
		t.Fatalf("expected help output to contain version %q, got:\n%s", "v9.9.9-test", output)
	}

	if !strings.Contains(output, "GoVM") {
		t.Fatal("expected help output to mention GoVM")
	}

	for _, want := range []string{
		"govm deps list",
		"govm deps check",
		"govm deps update",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestLoadTUISettings(t *testing.T) {
	defaultSettings := config.DefaultSettings()

	tests := []struct {
		name         string
		defaultPath  func() (string, error)
		load         func(string) (config.Settings, error)
		wantPath     string
		wantSettings config.Settings
		wantWarning  bool
	}{
		{
			name: "default path error warns and returns defaults with empty path",
			defaultPath: func() (string, error) {
				return "", errors.New("config dir unavailable")
			},
			load: func(string) (config.Settings, error) {
				t.Fatal("load should not be called when default path fails")
				return config.Settings{}, nil
			},
			wantPath:     "",
			wantSettings: defaultSettings,
			wantWarning:  true,
		},
		{
			name: "load error warns and returns defaults with resolved path",
			defaultPath: func() (string, error) {
				return "/tmp/govm-settings.json", nil
			},
			load: func(string) (config.Settings, error) {
				return config.Settings{}, errors.New("permission denied")
			},
			wantPath:     "/tmp/govm-settings.json",
			wantSettings: defaultSettings,
			wantWarning:  true,
		},
		{
			name: "successful load returns settings without warning",
			defaultPath: func() (string, error) {
				return "/tmp/govm-settings.json", nil
			},
			load: func(string) (config.Settings, error) {
				return config.Settings{
					DepsDisplay: config.DepsDisplayAll,
					Theme:       config.ThemeLight,
				}, nil
			},
			wantPath: "/tmp/govm-settings.json",
			wantSettings: config.Settings{
				DepsDisplay: config.DepsDisplayAll,
				Theme:       config.ThemeLight,
			},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer

			gotPath, gotSettings := loadTUISettings(&stderr, tt.defaultPath, tt.load)

			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotSettings != tt.wantSettings {
				t.Fatalf("settings = %#v, want %#v", gotSettings, tt.wantSettings)
			}
			hasWarning := strings.Contains(stderr.String(), "Warning:")
			if hasWarning != tt.wantWarning {
				t.Fatalf("warning presence = %t, want %t; stderr:\n%s", hasWarning, tt.wantWarning, stderr.String())
			}
		})
	}
}

func TestLoadTUISettingsMissingFileDoesNotWarn(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-settings.json")
	var stderr bytes.Buffer

	gotPath, gotSettings := loadTUISettings(&stderr, func() (string, error) {
		return missingPath, nil
	}, config.Load)

	if gotPath != missingPath {
		t.Fatalf("path = %q, want %q", gotPath, missingPath)
	}
	if gotSettings != config.DefaultSettings() {
		t.Fatalf("settings = %#v, want %#v", gotSettings, config.DefaultSettings())
	}
	if stderr.String() != "" {
		t.Fatalf("expected no warning for missing settings file, got:\n%s", stderr.String())
	}
}
