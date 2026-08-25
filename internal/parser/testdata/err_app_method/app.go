// Package app carries the three page methods on App, where none of them runs.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventAnnounced is "announced"
type EventAnnounced struct {
	Text string `json:"text"`
}

func (a *App) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (a *App) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

func (a *App) OnAnnounced(e EventAnnounced, sse datapages.SSE) error { return nil }

// Announce is an ordinary method of the application, which the framework leaves alone.
func (a *App) Announce(text string) string { return text }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnAnnounced(e EventAnnounced, sse datapages.SSE) error { return nil }
