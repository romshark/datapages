// Package app defines page GET handlers that return Datapages error sentinels.
package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

var errNoSuchItem = errors.New("no such item")

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

// PageBad is /bad-request
type PageBad struct{ App *App }

func (PageBad) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, datapages.ErrBadRequest
}

// PageDenied is /denied
type PageDenied struct{ App *App }

func (PageDenied) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, datapages.ErrForbidden
}

// PageGone is /gone
type PageGone struct{ App *App }

func (PageGone) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, datapages.ErrNotFound
}

// PageConflict is /conflict
type PageConflict struct{ App *App }

func (PageConflict) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, fmt.Errorf("%w: %w", datapages.ErrConflict, errNoSuchItem)
}

// PagePlain is /plain
type PagePlain struct{ App *App }

func (PagePlain) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}
