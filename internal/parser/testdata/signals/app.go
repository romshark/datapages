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

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSubmit is /form/submit
func (PageForm) POSTSubmit(
	r *http.Request,
	form datapages.Signals[struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}],
) error {
	_ = form
	return nil
}

// PageSearch is /search
type PageSearch struct{ App *App }

// GET with query + signals + reflectsignal
func (PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term string `query:"t" reflectsignal:"term"`
	}],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) (body datapages.Component, err error) {
	_ = query
	_ = signals
	return body, err
}

// POSTFilter is /search/filter
func (PageSearch) POSTFilter(
	r *http.Request,
	query datapages.Query[struct {
		Page int `query:"p"`
	}],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) error {
	_ = query
	_ = signals
	return nil
}
