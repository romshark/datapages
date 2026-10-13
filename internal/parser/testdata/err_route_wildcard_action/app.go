// Package app gives a page ending in a wildcard 3 actions: one past the
// wildcard, one at the page route and one without a path comment.
//
// The parser is expected to report each of them once.
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

// POSTDelete is /files/{rest...}/delete
func (PageFiles) POSTDelete(r *http.Request) error {
	return nil
}

// POSTSave is /files/{rest...}
func (PageFiles) POSTSave(r *http.Request) error {
	return nil
}

func (PageFiles) POSTMove(r *http.Request) error {
	return nil
}
