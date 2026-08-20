package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventPing is "ping"
type EventPing struct{}

// StateA is the state PageA tries to bind by pointer.
type StateA struct {
	Shared string
	Extra  string
}

// StateB is the state for PageB. Same pattern as StateA with different
// page-specific fields — this demonstrates a single generic abstract
// instantiated on two different concrete state types.
type StateB struct {
	Shared string
	Other  int
}

// Base is a generic abstract page parameterized by its concrete state
// type. Its handlers accept `state *S`; every concrete page that embeds
// `Base[StateX]` gets those handlers instantiated with `state *StateX`.
type Base[S any] struct{ App *App }

func (Base[S]) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
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

// PageA is /a
type PageA struct {
	App *App
	Base[*StateA]
}

func (PageA) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTExtend is /a/extend
func (PageA) POSTExtend(
	r *http.Request,
	state *StateA,
) error {
	state.Extra = "set-by-pagea"
	return nil
}

// PageB is /b
type PageB struct {
	App *App
	Base[StateB]
}

func (PageB) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTBump is /b/bump
func (PageB) POSTBump(
	r *http.Request,
	state *StateB,
) error {
	state.Other++
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
