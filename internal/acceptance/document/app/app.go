// Package app exercises the document a handler returns: the attributes of the
// html element. A page, a page action, an App action and the 404 page
// render their documents through different code paths.
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// shared is the document every page starts from,
// which an app would read from the request or the session.
func shared(_ *http.Request) datapages.DocumentAttrs {
	return datapages.DocumentAttrs{
		"lang": "en", "dir": "ltr", "class": "dark", "data-theme": "dark",
	}
}

// PageIndex is /
//
// It returns no document.
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageArticle is /article/{$}
type PageArticle struct{ App *App }

func (PageArticle) GET(r *http.Request) (
	docAttrs datapages.DocumentAttrs, body datapages.Component, err error,
) {
	docAttrs = shared(r)
	docAttrs["lang"] = "de"
	docAttrs["translate"] = "no"
	return docAttrs, templ.Raw("article"), nil
}

// PageEscaped is /escaped/{$}
type PageEscaped struct{ App *App }

func (PageEscaped) GET(_ *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return templ.Raw("escaped"), datapages.DocumentAttrs{"class": `a"><script>x`}, nil
}

// PageInvalid is /invalid/{$}
//
// Its document sets an attribute name that HTML compares without case.
type PageInvalid struct{ App *App }

func (PageInvalid) GET(_ *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return templ.Raw("invalid"),
		datapages.DocumentAttrs{"data-Theme": "dark"}, nil
}

// POSTRender is /render
func (PageIndex) POSTRender(r *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	docAttrs = shared(r)
	docAttrs["lang"], docAttrs["dir"] = "ar", "rtl"
	return templ.Raw("action body"), docAttrs, nil
}

// POSTAppRender is /app-render
func (*App) POSTAppRender(_ *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return templ.Raw("app action body"), datapages.DocumentAttrs{"class": "app"}, nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	docAttrs = shared(r)
	docAttrs["class"] = "missing"
	return templ.Raw("not found"), docAttrs, nil
}
