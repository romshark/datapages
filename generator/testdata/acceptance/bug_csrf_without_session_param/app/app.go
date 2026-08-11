// Package app reproduces a generator bug: an action that declares no session
// parameter is served without a CSRF token.
//
// The generated CSRF check runs inside the auth helper. Only handlers that
// declare session or sessionToken call that helper. An action that changes
// state without needing to know who the visitor is never reaches the check,
// even when the visitor holds a session and the server was started with
// WithCSRFProtection.
//
// A cross-site page can make the browser send that request with its cookies
// attached. Stopping exactly that is what CSRF tokens are for. The protection
// belongs to the request method, not to the handler's parameter list.
//
// See options.json. The case is expected to fail until every state-changing
// action of a visitor with a session is checked.
package app

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/a-h/templ"
)

type App struct {
	mu      sync.Mutex
	deleted int
}

// IssuedAt is fixed so the test can compute the token of the session.
var IssuedAt = time.Unix(1700000000, 0).UTC()

type Session struct {
	UserID   string
	IssuedAt time.Time
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(_ *http.Request, session Session) (
	body templ.Component, err error,
) {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">user=%s deleted=%d</pre>`,
		session.UserID, p.App.deleted)), nil
}

// POSTSignIn is /sign-in
func (PageIndex) POSTSignIn(
	_ *http.Request,
	signals struct {
		User string `json:"user"`
	},
) (newSession Session, err error) {
	return Session{UserID: signals.User, IssuedAt: IssuedAt}, nil
}

// POSTDelete is /delete
//
// The dangerous action. It does not need to know who the visitor is. It
// therefore declares no session and is never checked.
func (p PageIndex) POSTDelete(
	_ *http.Request,
	signals struct {
		Confirm bool `json:"confirm"`
	},
) error {
	if !signals.Confirm {
		return nil
	}
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.deleted++
	return nil
}
