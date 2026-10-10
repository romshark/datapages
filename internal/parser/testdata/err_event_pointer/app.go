// Package app has an event handler that takes its event by pointer.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventPing is "ping"
type EventPing struct {
	N int `json:"n"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnPing(
	event *EventPing, /* ErrSignatureEvHandEventPointer */
	sse datapages.SSE,
) error {
	return nil
}
