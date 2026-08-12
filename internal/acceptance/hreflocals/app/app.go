// Package app exercises path and query variables named after the locals the URL writer
// declares for itself: b, l, n, anyQuery, and the conversion variable of a query field.
package app

import (
	"fmt"
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
//
// b is what the writer calls its strings.Builder.
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path struct {
		B bool `path:"b"`
	},
) (body templ.Component, err error) {
	return templ.Raw(fmt.Sprintf(`<pre id="echo">b=%t</pre>`, path.B)), nil
}

// PageMix is /mix/{l}/{n}/{pageStr}
//
// l, n and anyQuery are the writer's other locals;
// pageStr is what it calls the conversion variable of the query field below.
type PageMix struct{ App *App }

func (PageMix) GET(
	_ *http.Request,
	path struct {
		L       int    `path:"l"`
		N       int    `path:"n"`
		PageStr string `path:"pageStr"`
	},
	query struct {
		AnyQuery string `query:"anyQuery"`
		Page     int    `query:"page"`
	},
) (body templ.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">l=%d n=%d pageStr=%s anyQuery=%s page=%d</pre>`,
		path.L, path.N, templ.EscapeString(path.PageStr),
		templ.EscapeString(query.AnyQuery), query.Page,
	)), nil
}
