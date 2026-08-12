// Package app exercises URL values carrying a URL separator: "/", "?", "&" and "#".
// Every page echoes what it parsed, which makes the round trip from
// builder to handler observable.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
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

// POSTRename is /item/{name}/rename
//
// An action URL carries values the same way a page URL does,
// and the expression a template holds is built by the action package.
func (PageItem) POSTRename(
	_ *http.Request,
	sse *datastar.ServerSentEventGenerator,
	path struct {
		Name string `path:"name"`
	},
	query struct {
		To string `query:"to"`
	},
) error {
	return sse.PatchElementTempl(echo("renamed %q to %q", path.Name, query.To))
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
