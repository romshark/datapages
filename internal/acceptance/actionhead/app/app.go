// Package app exercises an action that returns a head of its own,
// in an app that also has an app-wide head.
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

func (a *App) Head(_ *http.Request) datapages.Head {
	return templ.Raw(`<title>global</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// POSTRender is /render
func (PageIndex) POSTRender(_ *http.Request) (
	body datapages.Component, head datapages.Head, err error,
) {
	return templ.Raw("body"), templ.Raw(`<meta name="from" content="action">`), nil
}
