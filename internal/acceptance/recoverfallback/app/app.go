// Package app exercises a RecoverError that fails, on a response the server
// has already committed as an event stream.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// RecoverError cannot render anything. An application reports this when its
// own error UI is unavailable.
func (a *App) RecoverError(
	_ error,
	_ datapages.SSE,
) error {
	return errors.New("the error UI is unavailable")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError500 is /server-error
//
// The page a failed page load is answered with.
// A failed Datastar request goes to RecoverError instead.
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("server error"), nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(_ *http.Request) error {
	return datapages.ErrBadRequest
}
