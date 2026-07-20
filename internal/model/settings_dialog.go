package model

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

func renderDepsBackupLimitDialog(t styles.Theme, settings SettingsState, viewport viewportSize) string {
	errMessage := settings.DepsBackupLimitInputErr
	if errMessage == "" && settings.DepsBackupLimitInput.Err != nil {
		errMessage = settings.DepsBackupLimitInput.Err.Error()
	}

	lines := []string{
		t.DialogTitleStyle.Render("Set dependency backup limit"),
		"",
		t.DialogBodyStyle.Render(fmt.Sprintf(
			"Enter a whole number from %d to %d.",
			config.MinDepsBackupLimit,
			config.MaxDepsBackupLimit,
		)),
		t.DialogBodyStyle.Render(settings.DepsBackupLimitInput.View()),
	}
	if errMessage != "" {
		lines = append(lines, t.DialogWarningStyle.Render(errMessage))
	}
	lines = append(lines,
		"",
		t.DialogMutedStyle.Render("enter: save  esc: cancel"),
	)

	return renderDialog(
		t,
		lipgloss.JoinVertical(lipgloss.Left, lines...),
		errMessage != "",
		viewport,
	)
}
