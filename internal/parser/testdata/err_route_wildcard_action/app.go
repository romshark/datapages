// Package app gives a page ending in a wildcard four actions: one past the
// wildcard, one at the page route, one without a path comment and one
// adopted from an embedded type.
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

// Archiver declares an action that PageFiles adopts.
type Archiver struct{ App *App }

// POSTArchive is /files/archive
func (Archiver) POSTArchive(r *http.Request) error {
	return nil
}

// PageFiles is /files/{rest...}
type PageFiles struct {
	App *App
	Archiver
}

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
