package utils

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func DeleteVersion(version GoVersion) tea.Cmd {
	return func() tea.Msg {
		if !version.Installed {
			return ErrMsg(fmt.Errorf("version %s is not installed", version.Version))
		}

		if version.Active {
			return ErrMsg(fmt.Errorf("cannot delete active version - switch to another version first"))
		}

		if err := os.RemoveAll(version.Path); err != nil {
			return ErrMsg(fmt.Errorf("failed to delete version %s: %v", version.Version, err))
		}

		return DeleteCompleteMsg{Version: version.Version}
	}
}
