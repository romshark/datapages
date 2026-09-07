package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageSearch is /search
type PageSearch struct{ App *App }

func (PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term     string  `query:"t"`
		Category string  `query:"c"`
		Limit    int     `query:"l"`
		PriceMin int64   `query:"pmin"`
		MaxPrice float64 `query:"pmax"`
		InStock  bool    `query:"instock"`
		// A tag value is a URL parameter name and needn't be a Go identifier
		// nor free of what ends a Go string literal.
		PageSize int    `query:"page-size"`
		Quoted   string `query:"q\"uote"`
	}],
) (body datapages.Component, err error) {
	_ = query
	return body, err
}

// POSTFilter is /search/filter
func (PageSearch) POSTFilter(
	r *http.Request,
	params datapages.Query[struct {
		Page int `query:"p"`
	}],
) error {
	_ = params
	return nil
}
