// Package app has a page that holds a stream embed whose name is unexported.
//
// The composite literal is written in the generated package, which reaches an
// unexported name of the app package as little as any other importer does.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// base carries the stream hook the embedding page inherits.
type base struct{ App *App }

func (base) StreamOpen(_ *http.Request, _ datapages.StreamID) error { return nil }

// PageIndex is /
type PageIndex struct {
	App  *App
	base /* ErrPageEmbedUnexported */
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
