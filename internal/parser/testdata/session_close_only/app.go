//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
//
// It closes sessions without ever naming the session type.
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// POSTSubmit is /logout
func (PageIndex) POSTSubmit(r *http.Request) (
	closeSession bool,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/"}, nil
}
