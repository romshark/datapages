// Package app exercises CSRF protection of a state-changing action that
// declares neither session nor sessionToken.
package app

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	mu      sync.Mutex
	deleted int
}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">user=%s deleted=%d</pre>`,
		session.UserID(), p.App.deleted,
	)), nil
}

// POSTSignIn is /sign-in
func (PageIndex) POSTSignIn(
	_ *http.Request,
	signals struct {
		User string `json:"user"`
	},
) (newSession datapages.NewSession[struct{}], err error) {
	return datapages.NewSession[struct{}]{UserID: signals.User}, nil
}

// POSTDelete is /delete
//
// The dangerous action. It does not need to know who the visitor is.
// It therefore declares no session and is never checked.
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
