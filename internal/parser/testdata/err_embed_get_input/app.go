// Package app embeds a type whose GET takes an unsupported parameter in a
// page with a route variable.
//
// The parser is expected to report that parameter alone.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// Base declares the GET that PageItem adopts.
type Base struct{ App *App }

func (Base) GET(r *http.Request, n int) (body datapages.Component, err error) {
	return nil, nil
}

// PageItem is /item/{id}
type PageItem struct {
	App *App
	Base
}
