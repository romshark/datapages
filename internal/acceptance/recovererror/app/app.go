// Package app exercises RecoverError: the hook that turns a failed Datastar
// request into something the visitor can see.
//
// A script makes a Datastar request. An HTTP error status therefore reaches
// nothing the visitor is looking at. RecoverError answers on the SSE connection
// instead. That is how an application shows a toast rather than failing in silence.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// RecoverError renders the failure into the page the visitor is on.
func (a *App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	kind := "unknown"
	switch {
	case errors.Is(err, datapages.ErrBadRequest):
		kind = "bad request"
	case errors.Is(err, datapages.ErrNotFound):
		kind = "not found"
	case errors.Is(err, errUnrecoverable):
		// Returning an error from RecoverError is how an application says it
		// could not show anything. The server then falls back to a status.
		return errors.New("cannot render a toast for this")
	}
	return sse.PatchElement(
		templ.Raw(`<div id="toast">` + kind + `</div>`),
	)
}

var errUnrecoverable = errors.New("unrecoverable")

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

// PageError500 is /server-error
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">server error</pre>`), nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(_ *http.Request) error {
	return datapages.ErrBadRequest
}

// POSTMissing is /missing
func (PageIndex) POSTMissing(_ *http.Request) error {
	return datapages.ErrNotFound
}

// POSTPlain is /plain
func (PageIndex) POSTPlain(_ *http.Request) error {
	return errors.New("plain failure")
}

// POSTUnrecoverable is /unrecoverable
func (PageIndex) POSTUnrecoverable(_ *http.Request) error {
	return errUnrecoverable
}

// PageBoom is /boom
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}
