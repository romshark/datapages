package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/datapagesgen/href"
)

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (
	body templ.Component, redirect datapages.Redirect,
) {
	return nil, datapages.Redirect{URL: href.PageIndex(href.QueryPageIndex{})}
}
