//nolint:all

// Package app takes its types from packages named after ones app_gen.go imports.
// Unaliased, one identifier in the generated file would name two
// packages and the type would resolve to nothing.
//
// The app package itself is covered by pkgname_collides. This is every other
// package the model reaches: the session data type, and the path, query and
// signals field types.
package app

import (
	"net/http"

	"datapagestest/fixture/pkgname_foreign/mytypes"
	"datapagestest/fixture/pkgname_foreign/stream"
	"datapagestest/fixture/pkgname_foreign/strings"

	"github.com/romshark/datapages"
)

type App struct{}

// Session carries its data from a package named after runtime/stream.
type Session = datapages.Session[stream.Data]

// PageIndex is /
type PageIndex struct{ App *App }

// GET takes the session, which is how the parser finds the Session type.
func (PageIndex) GET(
	r *http.Request, session Session,
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID mytypes.ID `path:"id"`
	}],
	query datapages.Query[struct {
		Code  strings.Code  `query:"code"`
		Count strings.Count `query:"count"`
	}],
) (body datapages.Component, err error) {
	_, _ = path, query
	return body, err
}

// POSTSave is /item/{id}/save
func (PageItem) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		ID mytypes.ID `path:"id"`
	}],
	query datapages.Query[struct {
		Code strings.Code `query:"code"`
	}],
	signals datapages.Signals[struct {
		Code strings.Code `json:"code"`
	}],
) error {
	_, _, _ = path, query, signals
	return nil
}

// PageNamedInputs is /named
//
// A named path, query and signals type renders by its own name, which is the
// other half of the naming: the field types above render one by one.
type PageNamedInputs struct{ App *App }

// ItemQuery is a named query type of a package named after one app_gen.go imports.
type ItemQuery = struct {
	Code strings.Code `query:"code"`
}

func (PageNamedInputs) GET(
	r *http.Request,
	query datapages.Query[ItemQuery],
) (body datapages.Component, err error) {
	_ = query
	return body, err
}
