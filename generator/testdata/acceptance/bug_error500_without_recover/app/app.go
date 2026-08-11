// Package app reproduces a generator bug.
//
// An application supplies PageError500 to give a failed request a page in its
// own voice. The generated server renders it only from the error path that
// exists when the application also defines RecoverError. Without that method
// the generated httpErrIntern calls http.Error directly and PageError500 is
// reachable only by typing its own route.
//
// The two features are unrelated in the specification. RecoverError answers a
// Datastar request with a patch. PageError500 is what an ordinary page load
// renders when it fails. Nothing says one requires the other.
//
// See options.json. The case is expected to fail until a failed page load
// renders PageError500 whether or not RecoverError exists.
package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError500 is /server-error
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw(`<p id="msg">something went wrong on our side</p>`), nil
}

// PageBoom is /boom
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body templ.Component, err error) {
	return nil, errors.New("the page could not be built")
}
