//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
//
// It closes sessions without ever naming the session type.
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// POSTSubmit is /logout
func (PageIndex) POSTSubmit(r *http.Request) (
	closeSession bool,
	redirect string,
	err error,
) {
	return true, "/", nil
}
