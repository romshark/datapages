//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

/* ErrNewSessionNotSessionType: wrong type */

// POSTBadNew is /bad-new
func (PageIndex) POSTBadNew(
	r *http.Request,
) (newSession int, err error) {
	return 0, nil
}

/* ErrCloseSessionNotBool: wrong type */

// POSTBadClose is /bad-close
func (PageIndex) POSTBadClose(
	r *http.Request,
) (closeSession int, err error) {
	return 0, nil
}

/* ErrNewSessionWithSSE: newSession with sse */

// POSTNewWithSSE is /new-with-sse
func (PageIndex) POSTNewWithSSE(
	r *http.Request,
	sse datapages.SSE,
) (newSession datapages.NewSession[struct{}], err error) {
	_ = sse
	return newSession, nil
}

/* ErrCloseSessionWithSSE: closeSession with sse */

// POSTCloseWithSSE is /close-with-sse
func (PageIndex) POSTCloseWithSSE(
	r *http.Request,
	sse datapages.SSE,
) (closeSession bool, err error) {
	_ = sse
	return false, nil
}
