//nolint:all

// Package app names route wildcards net/http accepts and Go cannot give a
// function parameter. Without a report each one reaches the generator,
// which writes an href and action package the user's build refuses.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageKeyword is /keyword/{type}
type PageKeyword struct{ App *App }

func (PageKeyword) GET(
	r *http.Request,
	path datapages.Path[struct {
		T string `path:"type"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// PageBlank is /blank/{_}
type PageBlank struct{ App *App }

func (PageBlank) GET(
	r *http.Request,
	path datapages.Path[struct {
		V string `path:"_"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// PageAction is /action
type PageAction struct{ App *App }

func (PageAction) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSave is /action/{range}/save
func (PageAction) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		R string `path:"range"`
	}],
) error {
	_ = path
	return nil
}

// POSTAppSave is /app-save/{func}
func (a *App) POSTAppSave(
	r *http.Request,
	path datapages.Path[struct {
		F string `path:"func"`
	}],
) error {
	_ = path
	return nil
}
