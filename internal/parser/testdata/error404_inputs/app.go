// Package app has a 404 page whose GET takes every input render404 must supply:
// a path, a query, signals and a dispatcher. render404 also handles URLs that
// don't match the page's declared route.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventMissed is "missed"
type EventMissed struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageError404 is /not-found/{code}
type PageError404 struct{ App *App }

func (PageError404) GET(
	r *http.Request,
	path datapages.Path[struct {
		Code int `path:"code"`
	}],
	query datapages.Query[struct {
		Term string `query:"t"`
	}],
	signals datapages.Signals[struct {
		X string `json:"x"`
	}],
	dispatch datapages.Dispatcher[EventMissed],
) (body datapages.Component, err error) {
	return nil, nil
}
