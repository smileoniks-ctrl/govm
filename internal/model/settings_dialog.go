package model

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/config"
)

func renderDepsBackupLimitDialog(settings SettingsState, viewportWidth int) string {
	errMessage := settings.DepsBackupLimitInputErr
	if errMessage == "" && settings.DepsBackupLimitInput.Err != nil {
		errMessage = settings.DepsBackupLimitInput.Err.Error()
	}

	lines := []string{
		dialogTitleStyle.Render("Set dependency backup limit"),
		"",
		dialogBodyStyle.Render(fmt.Sprintf(
			"Enter a whole number from %d to %d.",
			config.MinDepsBackupLimit,
			config.MaxDepsBackupLimit,
		)),
		dialogBodyStyle.Render(settings.DepsBackupLimitInput.View()),
	}
	if errMessage != "" {
		lines = append(lines, dialogWarningStyle.Render(errMessage))
	}
	lines = append(lines,
		"",
		dialogMutedStyle.Render("enter: save  esc: cancel"),
	)

	return renderDialog(
		lipgloss.JoinVertical(lipgloss.Left, lines...),
		errMessage != "",
		[]int{viewportWidth},
	)
}
