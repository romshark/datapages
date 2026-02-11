//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

/* ErrRedirectNotString: redirect wrong type */

// POSTBadRedirect is /bad-redirect
//
// Action with redirect of wrong type.
func (PageIndex) POSTBadRedirect(
	r *http.Request,
) (redirect int, err error) {
	return 0, nil
}

/* ErrRedirectStatusNotInt: redirectStatus wrong type */

// POSTBadStatus is /bad-status
//
// Action with redirectStatus of wrong type.
func (PageIndex) POSTBadStatus(
	r *http.Request,
) (redirect string, redirectStatus string, err error) {
	return "", "", nil
}

/* ErrRedirectStatusWithoutRedirect: redirectStatus without redirect */

// POSTOrphanStatus is /orphan-status
//
// Action with redirectStatus but no redirect.
func (PageIndex) POSTOrphanStatus(
	r *http.Request,
) (redirectStatus int, err error) {
	return 303, nil
}
