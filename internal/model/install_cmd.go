package model

// This file owns the Bubbletea adapter for the install core.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// installTimeout is the maximum wall-clock budget granted to a single
// install run. A slow network or a large archive should not hang the
// TUI indefinitely.
const installTimeout = 30 * time.Minute

// installFunc is the contract between the TUI and the install core.
// It is satisfied by a bound install.Service method
// ((*Service).Install), and by fakes injected in tests.
type installFunc func(context.Context, install.Request) (install.Result, error)

// installSuccessMsg carries the whole install.Result for a completed
// installation, including any non-fatal warnings the core surfaced.
type installSuccessMsg install.Result

// installFailureMsg carries the requested version and the typed error
// that terminated the installation (typically an *install.Error whose
// Error() reports the failing stage).
type installFailureMsg struct {
	Version string
	Err     error
}

// installVersionCmd maps a catalog GoVersion to an install.Request and
// returns the tea.Cmd that runs the installation through the injected
// install function. The command always resolves to either an
// installSuccessMsg (carrying the whole install.Result) or an
// installFailureMsg (carrying the requested version and the typed
// error). A model whose installer was never bound surfaces a failure
// instead of panicking, which keeps bare test constructors safe.
func (m Model) installVersionCmd(v utils.GoVersion) tea.Cmd {
	return func() tea.Msg {
		req := install.Request{
			Version:  v.Version,
			Filename: v.Filename,
			URL:      v.URL,
			SHA256:   v.SHA256,
			Size:     v.Size,
		}
		if m.installGo == nil {
			return installFailureMsg{Version: req.Version, Err: errors.New("no installer configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
		defer cancel()
		result, err := m.installGo(ctx, req)
		if err != nil {
			return installFailureMsg{Version: req.Version, Err: err}
		}
		return installSuccessMsg(result)
	}
}

// installSuccessStatus renders the status text and kind for a completed
// installation. With no warnings it preserves the historical success
// text ("Successfully installed Go <version>"); with warnings it
// surfaces them as a single global warning status so the user is
// alerted that the toolchain installed but something needs attention
// (e.g. an unavailable checksum or a cleanup failure). The direct
// completion path and the reconciliation path both use this helper so
// the wording cannot drift between them.
func installSuccessStatus(version string, warnings []install.Warning) (text, kind string) {
	if len(warnings) == 0 {
		return fmt.Sprintf("Successfully installed Go %s", version), "success"
	}
	return fmt.Sprintf("Installed Go %s with warnings: %s", version, joinInstallWarnings(warnings)), "warning"
}

// joinInstallWarnings renders a warning slice into a single semicolon
// separated string for status display.
func joinInstallWarnings(warnings []install.Warning) string {
	parts := make([]string, len(warnings))
	for i, w := range warnings {
		parts[i] = w.Error()
	}
	return strings.Join(parts, "; ")
}
