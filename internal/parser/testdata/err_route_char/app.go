// Package app declares routes with double quotes, backslashes, backticks,
// question marks, number signs and percent signs that start no percent-encoding,
// which are rejected, and equivalent percent-encoded routes, which are accepted.
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

// PageQuote is /say"hi"
type PageQuote struct{ App *App } /* ErrRouteCharInvalid */

func (PageQuote) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageBackslash is /back\slash
type PageBackslash struct{ App *App } /* ErrRouteCharInvalid */

func (PageBackslash) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSave is /back\slash/save
func (PageBackslash) POSTSave(r *http.Request) error { return nil } /* ErrRouteCharInvalid */

// POSTQuote is /quote"
func (*App) POSTQuote(r *http.Request) error { return nil } /* ErrRouteCharInvalid */

// PageTick is /back`tick
type PageTick struct{ App *App } /* ErrRouteCharInvalid */

func (PageTick) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTPing is /back`tick/ping
func (PageTick) POSTPing(r *http.Request) error { return nil } /* ErrRouteCharInvalid */

// PageSearch is /search?q
type PageSearch struct{ App *App } /* ErrRouteCharInvalid */

func (PageSearch) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageSharp is /c#
type PageSharp struct{ App *App } /* ErrRouteCharInvalid */

func (PageSharp) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PagePercent is /100%
type PagePercent struct{ App *App } /* ErrRouteCharInvalid */

func (PagePercent) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTRate is /rate%2
//
// The percent-encoding is cut short.
func (*App) POSTRate(r *http.Request) error { return nil } /* ErrRouteCharInvalid */

// PageEncoded is /say%22hi%5C%60%3F%23%25
type PageEncoded struct{ App *App }

func (PageEncoded) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTPing is /say%22hi%5C%60%3F%23%25/ping
func (PageEncoded) POSTPing(r *http.Request) error { return nil }
