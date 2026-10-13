// Package app has a page that reaches 1 embedded type through 2 embedded
// fields at the same depth. Go treats the selectors of its handlers as ambiguous.
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

type Base struct{ App *App }

func (Base) StreamOpen(r *http.Request, id datapages.StreamID) error { return nil }

type Left struct {
	App *App
	Base
}

type Right struct {
	App *App
	Base
}

// PageDiamond is /diamond
//
// 1 embedded type is reached through 2 paths at the same depth.
type PageDiamond struct { /* ErrPageAmbiguousEmbed */
	App *App
	Left
	Right
}

func (PageDiamond) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
