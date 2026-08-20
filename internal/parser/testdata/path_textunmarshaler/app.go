// Package app has a path field of a type implementing encoding.TextUnmarshaler.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type ID struct{ v string }

func (i *ID) UnmarshalText(b []byte) error { i.v = string(b); return nil }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageItem is /item/{id}/{$}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID ID `path:"id"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}
