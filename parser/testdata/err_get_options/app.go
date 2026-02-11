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

/* ErrEnableBgStreamNotGET: not a GET handler */

// POSTBadStream is /bad-stream
func (PageIndex) POSTBadStream(
	r *http.Request,
) (enableBackgroundStreaming bool, err error) {
	return false, nil
}

/* ErrDisableRefreshNotGET: not a GET handler */

// POSTBadRefresh is /bad-refresh
func (PageIndex) POSTBadRefresh(
	r *http.Request,
) (disableRefreshAfterHidden bool, err error) {
	return false, nil
}

// PageBadType is /bad-type
type PageBadType struct{ App *App }

/* ErrEnableBgStreamNotBool: wrong type */

func (PageBadType) GET(
	r *http.Request,
) (
	body templ.Component,
	enableBackgroundStreaming int,
	err error,
) {
	return body, 0, nil
}

// PageBadType2 is /bad-type2
type PageBadType2 struct{ App *App }

/* ErrDisableRefreshNotBool: wrong type */

func (PageBadType2) GET(
	r *http.Request,
) (
	body templ.Component,
	disableRefreshAfterHidden int,
	err error,
) {
	return body, 0, nil
}
