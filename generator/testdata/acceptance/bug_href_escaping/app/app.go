// Package app reproduces a generator bug.
//
// The generated href package writes path and query values into a URL without
// escaping them. A value is whatever the application holds: an item name, a
// search term, a user's display name. The characters that separate a URL into
// parts are ordinary characters in all of those. A "/" in a path value adds a
// segment, an "&" in a query value adds a parameter, and a "#" ends the URL.
// The link then addresses something other than what it was built for.
//
// Neither SPECIFICATION.md nor the skill guide asks the application to escape
// values before passing them. The generated functions take typed values rather
// than strings that are already URLs. The application has nowhere to do it.
//
// See options.json. The case is expected to fail until href escapes what it
// writes.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

func echo(format string, args ...any) templ.Component {
	return templ.Raw("<pre id=\"echo\">" + fmt.Sprintf(format, args...) + "</pre>")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("index"), nil
}

// PageItem is /item/{name}
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path struct {
		Name string `path:"name"`
	},
) (body templ.Component, err error) {
	return echo("name=%q", path.Name), nil
}

// PageSearch is /search
type PageSearch struct{ App *App }

func (PageSearch) GET(
	_ *http.Request,
	query struct {
		Term string `query:"term"`
		Page int    `query:"page"`
	},
) (body templ.Component, err error) {
	return echo("term=%q page=%d", query.Term, query.Page), nil
}
