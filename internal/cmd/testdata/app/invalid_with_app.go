// Package app provides an invalid fixture that has a valid App type and
// PageIndex so the parser returns a non-nil model, but PageBadPath carries an
// ErrPageInvalidPathComm error.
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageBadPath is not-a-valid-path
type PageBadPath struct{ App *App }

func (PageBadPath) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}
