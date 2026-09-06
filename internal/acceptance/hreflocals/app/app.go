// Package app exercises path and query variables named after the locals the URL writer
// declares for itself: b, l, n, anyQuery, and the conversion variable of a query field.
// It also carries query tags that are no Go identifier.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageItem is /item/{b}
//
// b is what the writer calls its strings.Builder.
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path datapages.Path[struct {
		B bool `path:"b"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(`<pre id="echo">b=%t</pre>`, path.Values.B)), nil
}

// PageMix is /mix/{l}/{n}/{pageStr}
//
// l, n and anyQuery are the writer's other locals;
// pageStr is what it calls the conversion variable of the query field below.
type PageMix struct{ App *App }

func (PageMix) GET(
	_ *http.Request,
	path datapages.Path[struct {
		L       int    `path:"l"`
		N       int    `path:"n"`
		PageStr string `path:"pageStr"`
	}],
	query datapages.Query[struct {
		AnyQuery string `query:"anyQuery"`
		Page     int    `query:"page"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">l=%d n=%d pageStr=%s anyQuery=%s page=%d</pre>`,
		path.Values.L, path.Values.N, templ.EscapeString(path.Values.PageStr),
		templ.EscapeString(query.Values.AnyQuery), query.Values.Page,
	)), nil
}

// PageTags is /tags
//
// The query tags are URL parameter names and no Go identifier:
// the locals the writer declares for them are named after the fields.
type PageTags struct{ App *App }

func (PageTags) GET(
	_ *http.Request,
	query datapages.Query[struct {
		PageSize int    `query:"page-size"`
		Term     string `query:"q.term"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">page-size=%d q.term=%s</pre>`,
		query.Values.PageSize, templ.EscapeString(query.Values.Term),
	)), nil
}

// POSTSelect is /tags/select/{$}
//
// It answers 400 unless page-size arrived as 7,
// which is how the test reads the value the action URL carried.
func (PageTags) POSTSelect(
	_ *http.Request,
	query datapages.Query[struct {
		PageSize int `query:"page-size"`
	}],
) error {
	if query.Values.PageSize != 7 {
		return datapages.ErrBadRequest
	}
	return nil
}
