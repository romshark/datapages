// Package app exercises URL values carrying a URL separator: "/", "?", "&" and "#".
// Every page echoes what it parsed, which makes the round trip from
// builder to handler observable.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

func echo(format string, args ...any) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + fmt.Sprintf(format, args...) + "</pre>")
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("index"), nil
}

// PageItem is /item/{name}
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (body datapages.Component, err error) {
	return echo("name=%q", path.Values.Name), nil
}

// POSTRename is /item/{name}/rename
//
// An action URL carries values the same way a page URL does,
// and the expression a template holds is built by the action package.
func (PageItem) POSTRename(
	_ *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
	query datapages.Query[struct {
		To string `query:"to"`
	}],
) error {
	return sse.PatchElement(echo("renamed %q to %q", path.Values.Name, query.Values.To))
}

// EventRenamed is "renamed"
type EventRenamed struct{}

// OnRenamed turns PageItem into a stream page, which makes it render the
// data-init attribute that carries the path value.
func (PageItem) OnRenamed(event EventRenamed, sse datapages.SSE) error {
	return sse.PatchElement(echo("renamed"))
}

// PageSearch is /search
type PageSearch struct{ App *App }

func (PageSearch) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Term string `query:"term"`
		Page int    `query:"page"`
	}],
) (body datapages.Component, err error) {
	return echo("term=%q page=%d", query.Values.Term, query.Values.Page), nil
}
