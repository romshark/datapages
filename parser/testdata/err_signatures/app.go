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
type PageNoAppField struct {
	/* expect err: missing App *App */
}

func (PageNoAppField) GET() (body templ.Component, err error) {
	return nil, nil
}

// PageNoGET is /this-page-is-missing-a-get-handler
type PageNoGET struct {
	/* expect err: missing App *App */
}

// expect err: PageNoGET has no GET handler
