package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

func (*App) Head(r *http.Request) (body templ.Component, err error) {
	return head(), nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return pageIndex(), nil
}
