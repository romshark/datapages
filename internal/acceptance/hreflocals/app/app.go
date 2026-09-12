// Package app exercises path and query variables named after what the URL
// writer declares for itself: the locals b, l, n, anyQuery, the conversion
// variable of a query field, and the query and options parameters of the helpers.
// It also carries variables named after what the writer resolves at package scope,
// which is a shadow rather than a redeclaration,
// and query tags that are no Go identifier.
package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// Slug marshals itself, which is what routes a path value through the textOf
// helper the URL writers generate.
type Slug string

func (s Slug) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(string(s))), nil
}

func (s *Slug) UnmarshalText(b []byte) error {
	*s = Slug(b)
	return nil
}

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

// POSTStore is /mix/{l}/{n}/{pageStr}/store
//
// A path and a query in one action:
// the writer names its builder and lengths beside the path parameters.
func (PageMix) POSTStore(
	_ *http.Request,
	path datapages.Path[struct {
		L       int    `path:"l"`
		N       int    `path:"n"`
		PageStr string `path:"pageStr"`
	}],
	query datapages.Query[struct {
		AnyQuery string `query:"anyQuery"`
	}],
) error {
	if path.Values.L != 1 || query.Values.AnyQuery != "yes" {
		return datapages.ErrBadRequest
	}
	return nil
}

// PageParams is /params/{query}/{options}
//
// The href and action writers add a parameter of each name after the path parameters.
type PageParams struct{ App *App }

func (PageParams) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Query   string `path:"query"`
		Options string `path:"options"`
	}],
	query datapages.Query[struct {
		Term string `query:"t"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">query=%s options=%s t=%s</pre>`,
		templ.EscapeString(path.Values.Query),
		templ.EscapeString(path.Values.Options),
		templ.EscapeString(query.Values.Term),
	)), nil
}

// POSTSave is /params/{query}/{options}/save
func (PageParams) POSTSave(
	_ *http.Request,
	path datapages.Path[struct {
		Query   string `path:"query"`
		Options string `path:"options"`
	}],
) error {
	_ = path
	return nil
}

// PageLocals is /locals/{b}/{l}/{n}/{bl}/{al}
//
// Every local the action writer declares for itself, as path variables.
type PageLocals struct{ App *App }

func (PageLocals) GET(
	_ *http.Request,
	path datapages.Path[struct {
		B  string `path:"b"`
		L  string `path:"l"`
		N  string `path:"n"`
		BL string `path:"bl"`
		AL string `path:"al"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">b=%s l=%s n=%s bl=%s al=%s</pre>`,
		templ.EscapeString(path.Values.B), templ.EscapeString(path.Values.L),
		templ.EscapeString(path.Values.N), templ.EscapeString(path.Values.BL),
		templ.EscapeString(path.Values.AL),
	)), nil
}

// POSTSave is /locals/{b}/{l}/{n}/{bl}/{al}/save
func (PageLocals) POSTSave(
	_ *http.Request,
	path datapages.Path[struct {
		B  string `path:"b"`
		L  string `path:"l"`
		N  string `path:"n"`
		BL string `path:"bl"`
		AL string `path:"al"`
	}],
) error {
	_ = path
	return nil
}

// PageLen is /len/{len}
//
// len is the builtin the URL writers count the segments with.
// A parameter of that name leaves every len(s) in the body calling a string.
type PageLen struct{ App *App }

func (PageLen) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Len string `path:"len"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">len=%s</pre>`, templ.EscapeString(path.Values.Len),
	)), nil
}

// PageImports is /imports/{url}/{strings}/{strconv}/{textOf}
//
// Every wildcard is named after something the URL writers resolve at package scope:
// the net/url, strings and strconv packages, and the textOf helper. A parameter of
// that name hides the reference for the whole function body,
// which leaves url.PathEscape reading a string.
//
// The int reaches strconv and the Slug reaches textOf, which puts every one of
// those names under the parameter that would hide it.
type PageImports struct{ App *App }

func (PageImports) GET(
	_ *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings string `path:"strings"`
		Strconv int    `path:"strconv"`
		TextOf  Slug   `path:"textOf"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">url=%s strings=%s strconv=%d textOf=%s</pre>`,
		templ.EscapeString(path.Values.URL),
		templ.EscapeString(path.Values.Strings),
		path.Values.Strconv, templ.EscapeString(string(path.Values.TextOf)),
	)), nil
}

// POSTSave is /imports/{url}/{strings}/{strconv}/{textOf}/save
func (PageImports) POSTSave(
	_ *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings string `path:"strings"`
		Strconv int    `path:"strconv"`
		TextOf  Slug   `path:"textOf"`
	}],
	query datapages.Query[struct {
		Term string `query:"t"`
	}],
) error {
	if path.Values.Strconv != 3 || query.Values.Term != "x" {
		return datapages.ErrBadRequest
	}
	return nil
}

// PageExpr is /expr/{actionexpr}
//
// actionexpr is resolved by the action writers only:
// they call it around every expression they build, next to the path parameter.
type PageExpr struct{ App *App }

func (PageExpr) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Actionexpr string `path:"actionexpr"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(`<pre id="echo">actionexpr=%s</pre>`,
		templ.EscapeString(path.Values.Actionexpr))), nil
}

// POSTRun is /expr/{actionexpr}/run
func (PageExpr) POSTRun(
	_ *http.Request,
	path datapages.Path[struct {
		Actionexpr string `path:"actionexpr"`
	}],
) error {
	if path.Values.Actionexpr != "seven" {
		return datapages.ErrBadRequest
	}
	return nil
}
