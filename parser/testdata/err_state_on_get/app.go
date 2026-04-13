package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

type StateIndex struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	state *StateIndex, // not allowed on GET
) (body templ.Component, err error) {
	return nil, nil
}

// POSTDummy is /dummy
func (PageIndex) POSTDummy(
	r *http.Request,
	state *StateIndex,
) error {
	return nil
}

// StreamOpen anchors the state lifecycle so the fixture isolates the
// ErrStateOnGET error without also tripping ErrStateWithoutStream.
func (PageIndex) StreamOpen(r *http.Request, streamID uint64) error {
	return nil
}
