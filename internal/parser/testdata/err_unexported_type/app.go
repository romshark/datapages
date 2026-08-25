// Package app carries unexported types in a path, a query and a signals struct,
// none of which the generated package can name.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type (
	userID string
	filter struct {
		Term string `json:"term"`
	}
)

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageUser is /user/{id}
type PageUser struct{ App *App }

func (PageUser) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID userID `path:"id"`
	}],
	query datapages.Query[struct {
		Since userID `query:"since"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSearch is /search
func (PageIndex) POSTSearch(r *http.Request, signals datapages.Signals[struct {
	F []filter `json:"f"`
}],
) error {
	return nil
}
