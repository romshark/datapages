package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages/example/todolist/datapagesgen/href"
)

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, redirect string) {
	return nil, href.PageIndex(href.QueryPageIndex{})
}
