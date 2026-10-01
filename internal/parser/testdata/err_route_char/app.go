// Package app declares routes with double quotes and backslashes, which are
// rejected, and equivalent percent-encoded routes, which are accepted.
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

// PageEncoded is /say%22hi%5C
type PageEncoded struct{ App *App }

func (PageEncoded) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTPing is /say%22hi%5C/ping
func (PageEncoded) POSTPing(r *http.Request) error { return nil }
