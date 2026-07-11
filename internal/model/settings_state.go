package model

import (
	"errors"
	"strconv"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
)

const settingsRowCount = 3

type SettingsState struct {
	Values                  config.Settings
	Path                    string
	Cursor                  int
	EditingDepsBackupLimit  bool
	DepsBackupLimitInput    textinput.Model
	DepsBackupLimitInputErr string
}

func NewSettingsState(path string, settings config.Settings) SettingsState {
	state := SettingsState{
		Values: config.Normalize(settings),
		Path:   path,
	}
	state.DepsBackupLimitInput = newDepsBackupLimitInput(state.Values.Theme)
	return state
}

func (s *SettingsState) MoveUp() {
	if s.Cursor <= 0 {
		s.Cursor = settingsRowCount - 1
		return
	}
	s.Cursor--
}

func (s *SettingsState) MoveDown() {
	s.Cursor = (s.Cursor + 1) % settingsRowCount
}

func (s *SettingsState) OpenDepsBackupLimitInput() tea.Cmd {
	s.DepsBackupLimitInputErr = ""
	s.DepsBackupLimitInput.SetValue(strconv.Itoa(s.Values.DepsBackupLimit))
	s.DepsBackupLimitInput.CursorEnd()
	s.EditingDepsBackupLimit = true
	return s.DepsBackupLimitInput.Focus()
}

func (s *SettingsState) CloseDepsBackupLimitInput() {
	s.DepsBackupLimitInputErr = ""
	s.DepsBackupLimitInput.Blur()
	s.EditingDepsBackupLimit = false
}

func (s *SettingsState) ApplyTheme() {
	s.DepsBackupLimitInput.SetStyles(depsBackupLimitInputStyles(s.Values.Theme))
}

func newDepsBackupLimitInput(theme config.ThemeName) textinput.Model {
	input := textinput.New()
	input.Prompt = "Limit: "
	input.CharLimit = len(strconv.Itoa(config.MaxDepsBackupLimit))
	input.Validate = validateDepsBackupLimitInput
	input.SetStyles(depsBackupLimitInputStyles(theme))
	return input
}

func depsBackupLimitInputStyles(theme config.ThemeName) textinput.Styles {
	if theme == config.ThemeLight {
		return textinput.DefaultLightStyles()
	}
	return textinput.DefaultDarkStyles()
}

func validateDepsBackupLimitInput(value string) error {
	if value == "" {
		return errors.New("Enter a whole number.")
	}

	limit, err := strconv.Atoi(value)
	if err != nil {
		return errors.New("Enter a whole number.")
	}
	return config.ValidateDepsBackupLimit(limit)
}
