// Package app has GET handlers and actions that return a document,
// in any position among their results and under any name,
// and a page without one that writes the page cache.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	return nil, nil
}

// PageArticle is /article/{$}
type PageArticle struct{ App *App }

func (PageArticle) GET(r *http.Request) (
	d datapages.DocumentAttrs, body datapages.Component, err error,
) {
	return datapages.DocumentAttrs{"lang": "de"}, nil, nil
}

// POSTRender is /render
func (PageIndex) POSTRender(r *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return nil, datapages.DocumentAttrs{"dir": "rtl"}, nil
}

// POSTAppRender is /app-render
func (*App) POSTAppRender(r *http.Request) (
	docAttrs datapages.DocumentAttrs, body datapages.Component, err error,
) {
	return datapages.DocumentAttrs{"class": "dark"}, nil, nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return nil, datapages.DocumentAttrs{"class": "missing"}, nil
}
