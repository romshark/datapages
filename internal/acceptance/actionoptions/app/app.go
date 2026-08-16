// Package app exercises the generated action expression builders and the
// options a template passes them, including options that carry nothing.
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// POSTSave is /save
func (PageIndex) POSTSave(_ *http.Request) error { return nil }
