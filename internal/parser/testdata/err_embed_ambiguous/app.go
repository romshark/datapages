// Package app has pages that inherit handlers through more than one embedded
// field at the same depth. Go treats these selectors as ambiguous.
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

type SaverA struct{ App *App }

// POSTSave is /two/save
func (SaverA) POSTSave(r *http.Request) error { return nil }

type SaverB struct{ App *App }

// POSTSave is /two/save
func (SaverB) POSTSave(r *http.Request) error { return nil }

// PageTwo is /two
//
// Two embedded types at one depth define the same action.
type PageTwo struct { /* ErrPageAmbiguousEmbed */
	App *App
	SaverA
	SaverB
}

func (PageTwo) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

type Base struct{ App *App }

func (Base) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

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
// One embedded type is reached through two paths at one depth.
type PageDiamond struct { /* ErrPageAmbiguousEmbed */
	App *App
	Left
	Right
}

type Shallow struct{ App *App }

// POSTSave is /shadowed/save
func (Shallow) POSTSave(r *http.Request) error { return nil }

type Deep struct {
	App *App
	SaverA
}

// PageShadowed is /shadowed
//
// A shallower definition shadows the deeper one, which Go accepts.
type PageShadowed struct {
	App *App
	Shallow
	Deep
}

func (PageShadowed) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
