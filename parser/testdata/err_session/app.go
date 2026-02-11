//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

/* ErrSessionMissingUserID: no UserID field */

type Session struct {
	Name string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// PageBadType is /bad-type
type PageBadType struct{ App *App }

/* ErrSessionParamNotSessionType: wrong type */

func (PageBadType) GET(
	r *http.Request,
	session int,
) (body templ.Component, err error) {
	_ = session
	return body, err
}
