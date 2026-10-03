//nolint:all

// Package app gives four pages a session. PageAbout, the first by name,
// takes another data type than the other three, each of which conflicts with it.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type SessionData struct {
	Name string
}

// PageAbout is /about
type PageAbout struct{ App *App }

func (PageAbout) GET(
	r *http.Request,
	session datapages.Session[struct{}],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// PageIndex is /
type PageIndex struct{ App *App }

/* ErrSessionTypeConflict: different Data type than PageAbout */

func (PageIndex) GET(
	r *http.Request,
	session datapages.Session[SessionData],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// PageItem is /item
type PageItem struct{ App *App }

/* ErrSessionTypeConflict: different Data type than PageAbout */

func (PageItem) GET(
	r *http.Request,
	session datapages.Session[SessionData],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// PageOther is /other
type PageOther struct{ App *App }

/* ErrSessionTypeConflict: different Data type than PageAbout */

func (PageOther) GET(
	r *http.Request,
	session datapages.Session[SessionData],
) (body datapages.Component, err error) {
	_ = session
	return body, err
}
