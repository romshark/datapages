package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

func (*App) Head(r *http.Request) datapages.Head {
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
