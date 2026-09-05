package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// Base carries the GET every page inherits. Its path struct names a variable
// the embedding page's route does not declare.
type Base struct{ App *App }

func (Base) GET(
	_ *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"` /* ErrPathFieldNotInRoute */
	}],
) (body datapages.Component, err error) {
	_ = path
	return nil, nil
}

// PageIndex is /
type PageIndex struct {
	App *App
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageThing is /thing/{slug}
type PageThing struct {
	App *App
	Base
}
