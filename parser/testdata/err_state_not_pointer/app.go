package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

type StateIndex struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(
	r *http.Request,
	state StateIndex, // must be *StateIndex (pointer)
) error {
	return nil
}
