//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// StateIndex is the per-instance state of PageIndex.
type StateIndex struct {
	Filter string
}

// POSTAppExport is /app-export
//
// An app-level action that takes the state and returns a file.
func (*App) POSTAppExport(
	r *http.Request,
	state datapages.State[StateIndex],
) (datapages.File, error) {
	return datapages.File{}, nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTExport is /export
//
// A page action that takes the state and returns a file and no error.
func (PageIndex) POSTExport(
	r *http.Request,
	state datapages.State[StateIndex],
) datapages.File {
	return datapages.File{}
}
