// Package app declares two query fields that reflect the same signal.
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
		B string `query:"b" reflectsignal:"term"`
		C string `query:"c" reflectsignal:"term"` /* ErrQueryReflectSignalDuplicate */
	}],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) (body datapages.Component, err error) {
	_, _ = query, signals
	return body, err
}
