package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type TabContext struct{ Count int }

// PUTBump is /bump
//
// App-level action that takes per-tab state.Values. The runtime resolves the slot
// from the calling tab, which must sit on a page bound to TabContext.
// No page uses that type, which makes the action unreachable. The parser rejects this.
func (*App) PUTBump(
	r *http.Request,
	state datapages.State[TabContext],
) error {
	state.Values.Count++
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
