// Package app reproduces a generator bug.
//
// SPECIFICATION.md lists head templ.Component among the return values an
// action may declare. The generated handler assigns that return value to a
// local named head and then renders the app-wide head instead. The local stays
// unused and Go rejects it. No application can compile an action that returns
// its own head.
//
// The app below declares a global head as well. The case does not rest on the
// absence of one: the generated code fails either way.
//
// See options.json. The case is expected to fail until the writer either
// renders the returned head or the specification drops it.
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

func (a *App) Head(_ *http.Request) templ.Component {
	return templ.Raw(`<title>global</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// POSTRender is /render
func (PageIndex) POSTRender(_ *http.Request) (
	body, head templ.Component, err error,
) {
	return templ.Raw("body"), templ.Raw(`<meta name="from" content="action">`), nil
}
