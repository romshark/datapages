// Package app exercises a PageError500 that fails to render.
//
// A failed page load is answered by rendering PageError500. When that page
// fails too there is nothing left to render, and the server has to say so
// rather than ask the same page again. The page fails both ways a page can:
// by returning an error and by panicking.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	// Error500Panics makes PageError500 panic instead of returning an error.
	// A panic takes the deferred recovery, which is a second path into the
	// same rendering of the same page.
	Error500Panics bool
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError500 is /server-error
//
// The error UI is broken the way the rest of the application is.
type PageError500 struct{ App *App }

func (p PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	if p.App.Error500Panics {
		panic("the error page panicked")
	}
	return nil, errors.New("the error page could not be built")
}

// PageBoom is /boom
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}

// PagePanic is /panic
type PagePanic struct{ App *App }

func (PagePanic) GET(_ *http.Request) (body datapages.Component, err error) {
	panic("the page panicked")
}
