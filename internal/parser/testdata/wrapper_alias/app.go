// Package app types handler parameters with aliases of datapages.Path,
// datapages.Query, datapages.Signals and datapages.State.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Counter struct {
	N int
}

type (
	ItemPath = datapages.Path[struct {
		ID string `path:"id"`
	}]
	SearchQuery = datapages.Query[struct {
		Term string `query:"q"`
	}]
	FormSignals = datapages.Signals[struct {
		Text string `json:"text"`
	}]
	TabState = datapages.State[Counter]
)

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, query SearchQuery,
) (body datapages.Component, err error) {
	return nil, nil
}

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request, path ItemPath,
) (body datapages.Component, err error) {
	return nil, nil
}

func (PageItem) StreamOpen(
	r *http.Request, streamID datapages.StreamID, signals FormSignals,
	state TabState,
) error {
	return nil
}

// POSTSave is /item/{id}/save
func (PageItem) POSTSave(
	r *http.Request, path ItemPath, signals FormSignals, state TabState,
) error {
	return nil
}
