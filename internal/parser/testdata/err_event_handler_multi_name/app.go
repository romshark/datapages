// Package app has event handlers that declare two parameters in one field, and one
// that declares the SSE twice in two fields. Each name is a parameter of its own:
// a second SSE or stream ID is unsupported, and so is a string next to stateID.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// TabState holds per-tab filters.
type TabState struct {
	Filter string
}

// EventPing is "ping"
type EventPing struct{}

// EventPong is "pong"
type EventPong struct{}

// EventTick is "tick"
type EventTick struct{}

// EventTock is "tock"
type EventTock struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnPing(
	event EventPing,
	sse, sse2 datapages.SSE, /* ErrSignatureUnsupportedInput */
) error {
	return nil
}

func (PageIndex) OnPong(
	event EventPong,
	sse datapages.SSE,
	streamID, streamID2 datapages.StreamID, /* ErrSignatureUnsupportedInput */
) error {
	return nil
}

func (PageIndex) OnTick(
	event EventTick,
	sse datapages.SSE,
	state datapages.State[TabState],
	stateID, extra string, /* ErrSignatureUnsupportedInput */
) error {
	return nil
}

func (PageIndex) OnTock(
	event EventTock,
	sse datapages.SSE,
	again datapages.SSE, /* ErrSignatureUnsupportedInput */
) error {
	return nil
}
