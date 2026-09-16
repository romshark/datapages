package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

func (*App) Head(r *http.Request) datapages.Head {
	return nil
}

// StateIndex is the per-tab state of PageIndex.
type StateIndex struct {
	Filter string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTFilter is /filter
func (PageIndex) POSTFilter(
	r *http.Request,
	state datapages.State[StateIndex],
) error {
	state.Values.Filter = "all"
	return nil
}
