// Package app gives three actions a route ending in a wildcard: one on App,
// one adopted from an embedded type and one on a page.
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

// POSTUpload is /upload/{path...}
func (*App) POSTUpload(
	r *http.Request,
	path datapages.Path[struct {
		Path string `path:"path"`
	}],
) error {
	return nil
}

// Dropper declares an action that PageItem adopts.
type Dropper struct{ App *App }

// POSTDrop is /item/drop/{rest...}
func (Dropper) POSTDrop(
	r *http.Request,
	path datapages.Path[struct {
		Rest string `path:"rest"`
	}],
) error {
	return nil
}

// PageItem is /item
type PageItem struct {
	App *App
	Dropper
}

func (PageItem) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTAttach is /item/attach/{name...}
func (PageItem) POSTAttach(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) error {
	return nil
}
