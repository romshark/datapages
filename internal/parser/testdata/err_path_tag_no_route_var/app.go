//nolint:all
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

// EventPinged is "pinged"
type EventPinged struct {
	Text string `json:"text"`
}

/* ErrPathMissingRouteVar and ErrPathFieldNotInRoute: the tag is a typo.
   The stream makes the generator write the route with its path values,
   which is where the missed lookup by tag used to panic. */

// PageThing is /thing/{id}
type PageThing struct{ App *App }

func (PageThing) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"other"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

func (PageThing) OnPinged(_ EventPinged, _ datapages.SSE) error { return nil }
