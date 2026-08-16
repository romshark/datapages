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

/* ErrRedirectNotRedirectType: redirect wrong type */

// POSTBadRedirect is /bad-redirect
//
// Action with redirect of wrong type.
func (PageIndex) POSTBadRedirect(
	r *http.Request,
) (redirect int, err error) {
	return 0, nil
}

/* ErrRedirectNotRedirectType: redirect as a plain string */

// POSTStringRedirect is /string-redirect
//
// Action with the legacy string redirect.
func (PageIndex) POSTStringRedirect(
	r *http.Request,
) (redirect string, err error) {
	return "", nil
}
