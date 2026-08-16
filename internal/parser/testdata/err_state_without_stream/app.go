package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type CounterState struct{ Count int }

// PageIndex is /
//
// Page takes per-tab state via POSTAdd but has no StreamOpen, StreamClose,
// or OnXXX handler — without a stream there is nothing to anchor the
// state lifecycle to. The parser rejects this.
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTAdd is /add
func (PageIndex) POSTAdd(
	r *http.Request,
	state *CounterState,
) error {
	state.Count++
	return nil
}
