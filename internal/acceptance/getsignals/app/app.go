// Package app exercises what a page GET may take besides the request and what it
// may return besides its body: signals sent by the client, and a session it issues.
package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
)

type App struct{}

// Session is the app's session type.
type Session struct {
	UserID   string
	IssuedAt time.Time
}

// IssuedAt is fixed so that a test can predict the CSRF token of a session.
var IssuedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// PageIndex is /
//
// GET reads signals and the session, and echoes both.
type PageIndex struct{ App *App }

func (PageIndex) GET(
	_ *http.Request,
	session Session,
	signals struct {
		Term string `json:"term"`
		Page int    `json:"page"`
	},
) (body templ.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">term=%s page=%d user=%s</pre>`,
		templ.EscapeString(signals.Term), signals.Page,
		templ.EscapeString(session.UserID),
	)), nil
}

// PageEnter is /enter
//
// GET issues a session on a page load.
// The cookie has to be set before the page is written.
type PageEnter struct{ App *App }

func (PageEnter) GET(_ *http.Request) (
	body templ.Component,
	newSession Session,
	err error,
) {
	return templ.Raw(`<pre id="echo">entered</pre>`),
		Session{UserID: "alice", IssuedAt: IssuedAt}, nil
}

// POSTLeave is /leave
//
// An action that ends the session the caller holds.
func (PageIndex) POSTLeave(_ *http.Request, sessionToken string) (
	closeSession bool,
	err error,
) {
	return sessionToken != "", nil
}
