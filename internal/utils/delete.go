package utils

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/paths"
)

func DeleteVersion(version GoVersion) tea.Cmd {
	return func() tea.Msg {
		if !version.Installed {
			return ErrMsg(fmt.Errorf("version %s is not installed", version.Version))
		}

		if version.Active {
			return ErrMsg(fmt.Errorf("cannot delete active version - switch to another version first"))
		}

		resolver := paths.New()
		versionsDir, err := resolver.VersionsDir()
		if err != nil {
			return ErrMsg(fmt.Errorf("failed to resolve versions directory: %v", err))
		}
		if !paths.IsDirectChild(versionsDir, version.Path) ||
			filepath.Base(filepath.Clean(version.Path)) != "go"+version.Version {
			return ErrMsg(fmt.Errorf("refusing to delete invalid version path %q", version.Path))
		}

		if err := os.RemoveAll(version.Path); err != nil {
			return ErrMsg(fmt.Errorf("failed to delete version %s: %v", version.Version, err))
		}

		return DeleteCompleteMsg{Version: version.Version}
	}
}
