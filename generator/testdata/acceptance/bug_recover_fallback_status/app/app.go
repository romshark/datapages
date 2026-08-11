// Package app reproduces a generator bug.
//
// When a Datastar action fails, the generated httpErrIntern opens an SSE
// generator and hands the error to RecoverError. If RecoverError itself
// returns an error, the code falls back to http.Error with the status the
// sentinel chose.
//
// By then the SSE generator has written its own 200 OK. The fallback status is
// dropped, Go logs "superfluous response.WriteHeader call", and the status text
// is appended to the event stream instead. The client sees a successful request
// whose body ends in "Internal Server Error". That is the one outcome the
// fallback exists to prevent.
//
// See options.json. The case is expected to fail until the fallback runs
// before anything is written, or stops pretending it can set a status.
package app

import (
	"errors"
	"net/http"

	"dpacceptance/datapagesgen/httperr"
	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// RecoverError cannot render anything. An application reports this when its
// own error UI is unavailable.
func (a *App) RecoverError(
	_ error,
	_ *datastar.ServerSentEventGenerator,
) error {
	return errors.New("the error UI is unavailable")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError500 is /server-error
//
// The generator emits the RecoverError path only for an app that also has
// this page; see the bug_recover_requires_error500 case. It is here so that
// this case reaches the fallback at all.
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("server error"), nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(_ *http.Request) error {
	return httperr.BadRequest
}
