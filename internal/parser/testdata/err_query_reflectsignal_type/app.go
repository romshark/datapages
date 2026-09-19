// Package app binds a string query parameter to a bool signal.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	query datapages.Query[struct {
		Flag string `query:"flag" reflectsignal:"flag"` /* ErrQueryReflectSignalTypeMismatch */
	}],
	signals datapages.Signals[struct {
		Flag bool `json:"flag"`
	}],
) (body datapages.Component, err error) {
	_, _ = query, signals
	return body, err
}
