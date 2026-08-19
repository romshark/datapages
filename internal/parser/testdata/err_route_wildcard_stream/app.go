// Package app gives a page ending in a wildcard a stream.
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

// PageFiles is /files/{rest...}
type PageFiles struct{ App *App }

func (PageFiles) GET(
	r *http.Request,
	path datapages.Path[struct {
		Rest string `path:"rest"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}

func (PageFiles) StreamOpen(
	r *http.Request, streamID datapages.StreamID,
) error {
	return nil
}
