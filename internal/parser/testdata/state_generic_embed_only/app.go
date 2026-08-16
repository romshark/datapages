package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventPing is "ping"
type EventPing struct{}

// TabState is the per-instance state of PageIndex.
//
// The page reaches it only through Base[TabState]. No handler declared
// on the page itself mentions the type.
type TabState struct {
	Counter int
}

// Base is a generic abstract page parameterized by its state type.
type Base[S any] struct{ App *App }

func (Base[S]) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *S,
) error {
	return nil
}

func (Base[S]) OnPing(
	event EventPing,
	sse datapages.SSE,
	state *S,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base[TabState]
}

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
