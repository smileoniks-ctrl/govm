package model

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/services"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// LoadVersionsCmd wraps the synchronous LoadVersionCatalog operation
// in a tea.Cmd. This is the TUI adapter: it translates between the
// loader's domain result and Bubbletea's message protocol.
func LoadVersionsCmd(rt *services.Runtime) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		catalog, err := rt.Loader.Load(ctx)
		if err != nil {
			return utils.ErrMsg(err)
		}

		return utils.VersionsMsg(catalog.Versions)
	}
}
