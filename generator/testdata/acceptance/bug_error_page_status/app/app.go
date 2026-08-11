// Package app reproduces a generator bug.
//
// When an application supplies PageError404, the generated server renders that
// page for a URL no page claims and sends it with 200 OK. The response says
// "not found" in its body and "here is the page you asked for" in its status.
//
// A crawler indexes it, a cache stores it, a client that branches on the status
// treats the miss as a hit, and a monitor counting 404s sees none. The built-in
// response the server sends without a PageError404 does carry 404. Supplying
// the page is what loses the status.
//
// See options.json. The case is expected to fail until the rendered error
// pages carry their statuses.
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw(`<p id="msg">no such page</p>`), nil
}
