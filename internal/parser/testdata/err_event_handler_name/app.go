// Package app has event handlers whose names do not follow from their events,
// on a page and on a stream embed.
//
// PageShadow.OnB takes EventA and hides Base.OnB, which takes EventB.
// The generated call to Base.OnB would pass EventB to PageShadow.OnB.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventA is "a"
type EventA struct {
	Text string `json:"text"`
}

// EventB is "b"
type EventB struct {
	Text string `json:"text"`
}

// EventMessageSent is "message.sent"
type EventMessageSent struct {
	Text string `json:"text"`
}

// Base is a stream embed.
type Base struct{ App *App }

func (Base) OnB(event EventB, sse datapages.SSE) error { return nil }

func (Base) OnPing( // ErrEvHandNameMismatch
	event EventMessageSent, sse datapages.SSE,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnNewMessage( // ErrEvHandNameMismatch
	event EventMessageSent, sse datapages.SSE,
) error {
	return nil
}

// PageShadow is /shadow
type PageShadow struct {
	App *App
	Base
}

func (PageShadow) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageShadow) OnB( // ErrEvHandNameMismatch
	event EventA, sse datapages.SSE,
) error {
	return nil
}
