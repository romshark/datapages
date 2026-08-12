package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventPing is "ping"
type EventPing struct{}

// StateA is the state PageA binds through a chain of generic abstracts.
type StateA struct {
	Shared string
}

// StateB is the state PageB binds through a pointer embed.
type StateB struct {
	Other int
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
	sse *datastar.ServerSentEventGenerator,
	state *S,
) error {
	return nil
}

// Mid is a generic abstract page that embeds another one, passing its own
// type parameter down. A page embedding Mid[StateA] gets Base[StateA].
type Mid[S any] struct {
	App *App
	Base[S]
}

// PageA is /a
//
// Reaches its state type through two levels of generic abstract page.
type PageA struct {
	App *App
	Mid[StateA]
}

func (PageA) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// POSTSet is /a/set
func (PageA) POSTSet(
	r *http.Request,
	state *StateA,
) error {
	state.Shared = "set"
	return nil
}

// PageB is /b
//
// Embeds the abstract page by pointer, which Go allows and which names the
// same type.
type PageB struct {
	App *App
	*Base[StateB]
}

func (PageB) GET(r *http.Request) (body templ.Component, err error) {
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

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}
