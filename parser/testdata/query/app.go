package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageSearch is /search
type PageSearch struct{ App *App }

func (PageSearch) GET(
	r *http.Request,
	query struct {
		Term     string `query:"t"`
		Category string `query:"c"`
		Limit    int    `query:"l"`
		PriceMin int64  `query:"pmin"`
	},
) (body templ.Component, err error) {
	_ = query
	return body, err
}

// POSTFilter is /search/filter
func (PageSearch) POSTFilter(
	r *http.Request,
	query struct {
		Page int `query:"p"`
	},
) error {
	_ = query
	return nil
}
