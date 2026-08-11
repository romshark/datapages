// Package app reproduces a generator bug.
//
// WithHeaders returns the zero option when it is given no headers. The writer
// that assembles the options object does not skip it. It writes the option's
// key and value, both empty, and produces
//
//	@post('/save/', {: })
//
// That is not JavaScript. The attribute fails to parse in the browser and the
// action never runs.
//
// A template reaches this by computing its headers, the only reason to call
// WithHeaders with a map rather than a literal. Nothing reports it. The
// expression is a string, the Go code compiles, and the failure appears only
// in the browser.
//
// See options.json. The case is expected to fail until the writer skips
// options that carry nothing.
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

// POSTSave is /save
func (PageIndex) POSTSave(_ *http.Request) error { return nil }
