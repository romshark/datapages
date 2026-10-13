// Package app has a page that holds its stream embed as a pointer.
//
// Generated code writes a page as a composite literal of values.
// A pointer field takes the address of one, which the literal cannot give it,
// and a nil pointer panics in every promoted handler.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// Base carries the stream hook the embedding page inherits.
type Base struct{ App *App }

func (Base) StreamOpen(_ *http.Request, _ datapages.StreamID) error { return nil }

// PageIndex is /
type PageIndex struct {
	App   *App
	*Base /* ErrPageEmbedPointer */
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
