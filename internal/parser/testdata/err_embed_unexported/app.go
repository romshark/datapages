// Package app embeds an abstract page whose name is unexported.
//
// The composite literal is written in the generated package, which reaches an
// unexported name of the app package as little as any other importer does.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// base carries the GET the embedding page inherits.
type base struct{ App *App }

func (base) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageIndex is /
type PageIndex struct {
	App  *App
	base /* ErrPageEmbedUnexported */
}
