// Package app declares pages besides PageIndex at "/", which are rejected:
// net/http routes "/" to the more specific pattern, which PageIndex isn't.
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

// PageHome is /
type PageHome struct{ App *App } /* ErrRouteConflict */

func (PageHome) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageRoot is /{$}
type PageRoot struct{ App *App } /* ErrRouteConflict */

func (PageRoot) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageUser is /{name}
//
// A variable at the root claims paths below "/", not "/" itself.
type PageUser struct{ App *App }

func (PageUser) GET(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}
