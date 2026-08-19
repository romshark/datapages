// Package app has a query field of a type it names itself.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Slug string

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	query datapages.Query[struct {
		S Slug `query:"s"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}
