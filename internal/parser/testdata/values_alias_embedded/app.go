// Package app uses aliases and embedded fields in path, query and signals type
// arguments. Generated code must resolve the aliases and keep the embedded
// fields embedded.
package app

import (
	"net/http"
	"time"

	"github.com/romshark/datapages"
)

type App struct{}

type SearchQuery struct {
	Term string `query:"t"`
}

// SQ aliases a defined type.
type SQ = SearchQuery

type ItemPath struct {
	ID string `path:"id"`
}

// itemPath is unexported, but its target is exported.
type itemPath = ItemPath

type Form struct {
	Name string `json:"name"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, query datapages.Query[SQ],
) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSubmit is /submit
func (PageIndex) POSTSubmit(
	r *http.Request,
	signals datapages.Signals[struct {
		Form      `json:"form"`
		time.Time `json:"t"`
	}],
) error {
	return nil
}

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[itemPath],
	query datapages.Query[struct {
		time.Time `query:"t"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}
