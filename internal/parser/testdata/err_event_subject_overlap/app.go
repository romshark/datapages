// Package app declares an event whose subject falls under another's.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventNotify is "notify"
//
// The subject field appends a token,
// so this event occupies every subject under "notify.".
type EventNotify struct {
	Room datapages.Subject

	Text string `json:"text"`
}

// EventNotifyUser is "notify.user"
type EventNotifyUser struct {
	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnNotify(event EventNotify, sse datapages.SSE) error { return nil }

func (PageIndex) OnNotifyUser(event EventNotifyUser, sse datapages.SSE) error { return nil }
