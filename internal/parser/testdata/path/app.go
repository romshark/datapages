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

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// POSTUpdate is /item/{id}/update
func (PageItem) POSTUpdate(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
) error {
	_ = path
	return nil
}

// PageProduct is /product/{id}/{version}
type PageProduct struct{ App *App }

func (PageProduct) GET(
	r *http.Request,
	path struct {
		ID      int32   `path:"id"`
		Version float64 `path:"version"`
	},
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// PageToggle is /toggle/{active}
type PageToggle struct{ App *App }

func (PageToggle) GET(
	r *http.Request,
	path struct {
		Active bool `path:"active"`
	},
) (body datapages.Component, err error) {
	_ = path
	return body, err
}
