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
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// POSTUpdate is /item/{id}/update
func (PageItem) POSTUpdate(
	r *http.Request,
	vars datapages.Path[struct {
		ID string `path:"id"`
	}],
) error {
	_ = vars
	return nil
}

// PageProduct is /product/{id}/{version}
type PageProduct struct{ App *App }

func (PageProduct) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID      int32   `path:"id"`
		Version float64 `path:"version"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// NamedPath is a named path struct, accepted like an anonymous one.
type NamedPath struct {
	ID string `path:"id"`
}

// PageNamed is /named/{id}
type PageNamed struct{ App *App }

func (PageNamed) GET(
	r *http.Request,
	path datapages.Path[NamedPath],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// PageToggle is /toggle/{active}
type PageToggle struct{ App *App }

func (PageToggle) GET(
	r *http.Request,
	path datapages.Path[struct {
		Active bool `path:"active"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}
