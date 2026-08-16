// Package app exercises a PageError500 that fails to render.
//
// A failed page load is answered by rendering PageError500. When that page
// fails too there is nothing left to render, and the server has to say so
// rather than ask the same page again.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError500 is /server-error
//
// The error UI is broken the way the rest of the application is.
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the error page could not be built")
}

// PageBoom is /boom
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}
