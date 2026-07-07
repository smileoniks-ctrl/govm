package model

import "github.com/smileoniks-ctrl/govm/internal/config"

const settingsRowCount = 2

type SettingsState struct {
	Values config.Settings
	Path   string
	Cursor int
}

func NewSettingsState(path string, settings config.Settings) SettingsState {
	return SettingsState{
		Values: config.Normalize(settings),
		Path:   path,
	}
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
