package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (
	body templ.Component, err error,
	err2 error, // expect err: multiple error return values
) {
	return nil, nil, nil
}

// PageNoAppField is /no-app-field
type PageNoAppField struct{}

func (PageNoAppField) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}
