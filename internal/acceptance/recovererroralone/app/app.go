// Package app exercises RecoverError in an app that supplies no PageError500.
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
