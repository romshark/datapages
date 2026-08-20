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

// PageBadGETOutput is /bad-get-output
type PageBadGETOutput struct{ App *App }

/* ErrSignatureUnsupportedOutput: nope is not a valid return name */

func (PageBadGETOutput) GET(
	r *http.Request,
) (body datapages.Component, nope bool, err error) {
	return body, false, err
}

/* ErrSignatureUnsupportedOutput: foo is not a valid return name */

// POSTBadActionOutput is /bad-action-output
func (PageIndex) POSTBadActionOutput(
	r *http.Request,
) (foo int, err error) {
	return 0, nil
}
