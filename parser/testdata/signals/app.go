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

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// POSTSubmit is /form/submit
func (PageForm) POSTSubmit(
	r *http.Request,
	signals struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	},
) error {
	_ = signals
	return nil
}

// PageSearch is /search
type PageSearch struct{ App *App }

// GET with query + signals + reflectsignal
func (PageSearch) GET(
	r *http.Request,
	query struct {
		Term string `query:"t" reflectsignal:"term"`
	},
	signals struct {
		Term string `json:"term"`
	},
) (body templ.Component, err error) {
	_ = query
	_ = signals
	return body, err
}

// POSTFilter is /search/filter
func (PageSearch) POSTFilter(
	r *http.Request,
	query struct {
		Page int `query:"p"`
	},
	signals struct {
		Term string `json:"term"`
	},
) error {
	_ = query
	_ = signals
	return nil
}
