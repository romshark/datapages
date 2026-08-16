// Package app exercises PageError404:
// the page and the status a URL that no page claims is answered with.
package app

import (
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

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<p id="msg">no such page</p>`), nil
}
