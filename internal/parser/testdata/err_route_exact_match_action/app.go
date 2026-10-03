// Package app gives a page ending in {$} four actions: one below its route,
// one at it, one without a path comment and one adopted from an embedded type.
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

// Archiver declares an action that PageSettings adopts.
type Archiver struct{ App *App }

// POSTArchive is /settings/archive
func (Archiver) POSTArchive(r *http.Request) error {
	return nil
}

// PageSettings is /settings/{$}
type PageSettings struct {
	App *App
	Archiver
}

func (PageSettings) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSave is /settings/save
func (PageSettings) POSTSave(r *http.Request) error {
	return nil
}

// POSTReset is /settings/{$}
func (PageSettings) POSTReset(r *http.Request) error {
	return nil
}

func (PageSettings) POSTMove(r *http.Request) error {
	return nil
}
