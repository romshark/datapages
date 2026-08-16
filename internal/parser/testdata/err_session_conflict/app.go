//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type SessionData struct {
	Name string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	session datapages.Session[SessionData],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// PageOther is /other
type PageOther struct{ App *App }

/* ErrSessionTypeConflict: different Data type than PageIndex */

func (PageOther) GET(
	r *http.Request,
	session datapages.Session[struct{}],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}
