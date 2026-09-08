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

/* ErrRouteConflict and ErrPathFieldNotInRoute: the route opens a brace and
   never closes it. The reflectsignal field makes the generator write the
   route into the data-effect attribute, which is where the scan used to
   slice out of range. */

// PageThing is /thing/{id
type PageThing struct{ App *App }

func (PageThing) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	query datapages.Query[struct {
		Term string `query:"t" reflectsignal:"term"`
	}],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) (body datapages.Component, err error) {
	_, _, _ = path, query, signals
	return body, err
}
