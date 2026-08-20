//nolint:all

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
) (body datapages.Component, err error) {
	return body, err
}

// PageBadType is /bad-type
type PageBadType struct{ App *App }

/* ErrSignatureUnsupportedInput: not a datapages.Session */

func (PageBadType) GET(
	r *http.Request,
	session int,
) (body datapages.Component, err error) {
	_ = session
	return body, err
}
