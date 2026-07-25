package model

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/services"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// catalogLoadPurpose identifies why a catalog load was requested. The
// purpose is carried with the request so callers can correlate refreshes
// without inferring intent from the request ID.
type catalogLoadPurpose string

const (
	catalogLoadPurposeInitial catalogLoadPurpose = "initial"
	catalogLoadPurposeRefresh catalogLoadPurpose = "refresh"
)

// catalogLoadRequest identifies one catalog load operation.
type catalogLoadRequest struct {
	ID      uint64
	Purpose catalogLoadPurpose
}

// catalogLoadedMsg carries the result of one catalog load operation.
type catalogLoadedMsg struct {
	RequestID uint64
	Versions  []utils.GoVersion
}

// catalogLoadFailedMsg carries the error from one catalog load operation.
type catalogLoadFailedMsg struct {
	RequestID uint64
	Err       error
}

// LoadVersionsCmd wraps the synchronous LoadVersionCatalog operation
// in a tea.Cmd. This is the TUI adapter: it translates between the
// loader's domain result and Bubbletea's message protocol.
func LoadVersionsCmd(rt *services.Runtime, request catalogLoadRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if rt == nil || rt.Loader == nil {
			return catalogLoadFailedMsg{
				RequestID: request.ID,
				Err:       errors.New("no loader configured"),
			}
		}
		catalog, err := rt.Loader.Load(ctx)
		if err != nil {
			return catalogLoadFailedMsg{RequestID: request.ID, Err: err}
		}

		return catalogLoadedMsg{RequestID: request.ID, Versions: catalog.Versions}
	}
}
