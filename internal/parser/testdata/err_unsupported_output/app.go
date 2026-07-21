//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageBadGETOutput is /bad-get-output
type PageBadGETOutput struct{ App *App }

/* ErrSignatureUnsupportedOutput: nope is not a valid return name */

func (PageBadGETOutput) GET(
	r *http.Request,
) (body templ.Component, nope bool, err error) {
	return body, false, err
}

// PageUppercaseBody is /uppercase-body
type PageUppercaseBody struct{ App *App }

/* ErrSignatureGETBodyWrongName: Body != body */

func (PageUppercaseBody) GET(
	r *http.Request,
) (Body templ.Component, err error) {
	return Body, err
}

/* ErrSignatureUnsupportedOutput: foo is not a valid return name */

// POSTBadActionOutput is /bad-action-output
func (PageIndex) POSTBadActionOutput(
	r *http.Request,
) (foo int, err error) {
	return 0, nil
}
