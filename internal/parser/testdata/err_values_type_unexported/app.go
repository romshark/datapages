// Package app uses unexported defined types as path, query and signals type arguments.
// Generated code cannot name them from another package.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type signalsIndex struct {
	Name string `json:"name"`
}

type queryItem struct {
	Term string `query:"t"`
}

type pathItem struct {
	ID string `path:"id"`
}

// QueryAlias is exported but resolves to an unexported defined type.
type QueryAlias = queryItem

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	query datapages.Query[QueryAlias], /* ErrValuesTypeUnexported */
) (body datapages.Component, err error) {
	return nil, nil
}

// POSTSubmit is /submit
func (PageIndex) POSTSubmit(
	r *http.Request,
	signals datapages.Signals[signalsIndex], /* ErrValuesTypeUnexported */
) error {
	return nil
}

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[pathItem], /* ErrValuesTypeUnexported */
	query datapages.Query[queryItem], /* ErrValuesTypeUnexported */
) (body datapages.Component, err error) {
	return nil, nil
}
