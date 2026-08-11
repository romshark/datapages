// Package app reproduces a generator bug.
//
// The generated httpErrIntern calls RecoverError only when the model has both
// RecoverError and PageError500 (see writeAppErrHelpers). An application that
// defines RecoverError alone gets the plain http.Error path: the method is
// compiled, never called, and never reported as unused.
//
// The two features answer different questions. RecoverError decides what a
// failed Datastar request shows the visitor, PageError500 decides what a
// failed page load renders. SPECIFICATION.md introduces them separately and
// makes neither depend on the other. The mirror image of this case is
// bug_error500_without_recover.
//
// See options.json. The case is expected to fail until RecoverError is called
// for an app that defines it.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// RecoverError renders the failure into the page the visitor is on.
func (a *App) RecoverError(
	_ error,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(`<div id="toast">something went wrong</div>`))
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// POSTFail is /fail
func (PageIndex) POSTFail(_ *http.Request) error {
	return errors.New("the action failed")
}
