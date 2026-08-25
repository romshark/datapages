// Package app declares a page at the URL the stream of another page is served under.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventTicked is "ticked"
type EventTicked struct {
	At int64 `json:"at"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnTicked(e EventTicked, sse datapages.SSE) error { return nil }

// PageStream is /_$
type PageStream struct{ App *App }

func (PageStream) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
