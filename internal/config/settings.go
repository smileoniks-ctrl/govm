package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type DepsDisplayMode string

const (
	DepsDisplayDirect DepsDisplayMode = "direct"
	DepsDisplayAll    DepsDisplayMode = "all"
)

type ThemeName string

const (
	ThemeCurrent ThemeName = "current"
	ThemeLight   ThemeName = "light"
)

const tempFilePrefix = ".settings-"

type Settings struct {
	DepsDisplay DepsDisplayMode `json:"depsDisplay"`
	Theme       ThemeName       `json:"theme"`
}

func DefaultSettings() Settings {
	return Settings{
		DepsDisplay: DepsDisplayDirect,
		Theme:       ThemeCurrent,
	}
}

func Normalize(settings Settings) Settings {
	if settings.DepsDisplay != DepsDisplayDirect && settings.DepsDisplay != DepsDisplayAll {
		settings.DepsDisplay = DepsDisplayDirect
	}
	if settings.Theme != ThemeCurrent && settings.Theme != ThemeLight {
		settings.Theme = ThemeCurrent
	}
	return settings
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "govm", "settings.json"), nil
}

func Load(path string) (Settings, error) {
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return Settings{}, err
		}
		path = defaultPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultSettings(), nil
		}
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}
	return Normalize(settings), nil
}

func Save(path string, settings Settings) error {
	return saveWithRename(path, settings, os.Rename)
}

func saveWithRename(path string, settings Settings, rename func(string, string) error) error {
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return fmt.Errorf("resolve default settings path: %w", err)
		}
		path = defaultPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(Normalize(settings), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(dir, tempFilePrefix)
	if err != nil {
		return fmt.Errorf("create temp settings file: %w", err)
	}
	tempPath := tempFile.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		if closeErr := tempFile.Close(); closeErr != nil {
			return errors.Join(fmt.Errorf("write temp settings file: %w", err), fmt.Errorf("close temp settings file: %w", closeErr))
		}
		return fmt.Errorf("write temp settings file: %w", err)
	}
	if err := tempFile.Chmod(0o644); err != nil {
		if closeErr := tempFile.Close(); closeErr != nil {
			return errors.Join(fmt.Errorf("set temp settings file mode: %w", err), fmt.Errorf("close temp settings file: %w", closeErr))
		}
		return fmt.Errorf("set temp settings file mode: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp settings file: %w", err)
	}
	if err := rename(tempPath, path); err != nil {
		return fmt.Errorf("rename settings file: %w", err)
	}
	renamed = true
	return nil
}
