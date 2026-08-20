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

/* ErrEnableBgStreamNotGET: not a GET handler */

// POSTBadStream is /bad-stream
func (PageIndex) POSTBadStream(
	r *http.Request,
) (enableBackgroundStreaming datapages.EnableBackgroundStreaming, err error) {
	return false, nil
}

/* ErrDisableRefreshNotGET: not a GET handler */

// POSTBadRefresh is /bad-refresh
func (PageIndex) POSTBadRefresh(
	r *http.Request,
) (disableRefreshAfterHidden datapages.DisableRefreshAfterHidden, err error) {
	return false, nil
}

// PageBadType is /bad-type
type PageBadType struct{ App *App }

/* ErrSignatureUnsupportedOutput: wrong type */

func (PageBadType) GET(
	r *http.Request,
) (
	body datapages.Component,
	enableBackgroundStreaming int,
	err error,
) {
	return body, 0, nil
}

// PageBadType2 is /bad-type2
type PageBadType2 struct{ App *App }

/* ErrSignatureUnsupportedOutput: wrong type */

func (PageBadType2) GET(
	r *http.Request,
) (
	body datapages.Component,
	disableRefreshAfterHidden int,
	err error,
) {
	return body, 0, nil
}
