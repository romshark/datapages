// Package app reproduces a generator bug.
//
// The URL writer declares its own locals inside the function it generates,
// among them a strings.Builder named b. A path variable of the same name
// becomes a parameter of that function and the two collide. The generated href
// package then does not compile. Nothing about the name b is unusual, and
// nothing in the app package can be written differently to avoid it.
//
// See options.json. The case is expected to fail until the writer names its
// locals out of reach of user-chosen path and query variables.
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

// PageItem is /item/{b}
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path struct {
		B bool `path:"b"`
	},
) (body templ.Component, err error) {
	_ = path
	return templ.Raw("item"), nil
}
