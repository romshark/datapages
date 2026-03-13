//nolint:all
package app

import (
	"net/http"

	"datapagestest/fixture/ok_templ_href_dot/template"
	"github.com/a-h/templ"
)

type App struct{}

type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return template.TemplatePageIndex(), nil
}

type Page struct{ App *App }

func (Page) GET(r *http.Request) (body templ.Component, err error) {
	return template.TemplatePageProfile(), nil
}
