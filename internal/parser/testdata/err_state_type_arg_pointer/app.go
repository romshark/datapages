package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateA struct{ Shared string }

// Base is an abstract page parameterized by its state type. Type parameters on
// a generated type are rejected on their own; the pointer type argument at the
// embed site below is the second error, and the one this fixture is about.
type Base[S any] struct{ App *App }

func (Base[S]) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[S],
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

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
