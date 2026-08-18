// Package app declares two pages at one URL.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageFoo is /foo/{$}
type PageFoo struct{ App *App }

func (PageFoo) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageBar is /foo/{$}
type PageBar struct{ App *App }

func (PageBar) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
