// Package app exercises what happens when a handler fails: the status the
// client is given, and the pages the app supplies for the two statuses it can take over.
package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

func echo(s string) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("index"), nil
}

// PageError404 is /not-found
//
// The page the app supplies for a URL no page claims.
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return echo("not found: " + r.URL.Path), nil
}

// PageError500 is /server-error
//
// The page the app supplies for a handler that failed.
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("server error"), nil
}

// PageBoom is /boom
type PageBoom struct{ App *App }

// GET fails, the way a page load fails when its data cannot be read.
func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}

// POSTPlain is /boom/plain
func (PageBoom) POSTPlain(_ *http.Request) error {
	return errors.New("something went wrong")
}

// POSTBad is /boom/bad
func (PageBoom) POSTBad(_ *http.Request) error {
	return datapages.ErrBadRequest
}

// POSTForbidden is /boom/forbidden
func (PageBoom) POSTForbidden(_ *http.Request) error {
	return datapages.ErrForbidden
}

// POSTNotFound is /boom/not-found
func (PageBoom) POSTNotFound(_ *http.Request) error {
	return datapages.ErrNotFound
}

// POSTConflict is /boom/conflict
func (PageBoom) POSTConflict(_ *http.Request) error {
	return datapages.ErrConflict
}

// POSTWrapped is /boom/wrapped
//
// The form SPECIFICATION.md recommends for
// keeping the original error while choosing the status.
func (PageBoom) POSTWrapped(_ *http.Request) error {
	return fmt.Errorf("%w: %w", datapages.ErrNotFound, errors.New("no such item"))
}
