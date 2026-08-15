package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

func (*App) Head(r *http.Request) templ.Component {
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}
