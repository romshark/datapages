// Package app gives a page ending in {$} an event handler, which opens a stream.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// EventPinged is "pinged"
type EventPinged struct{}

// PageUser is /user/{name}/{$}
type PageUser struct{ App *App }

func (PageUser) GET(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}

func (PageUser) OnPinged(event EventPinged, sse datapages.SSE) error {
	return nil
}
