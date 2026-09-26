// Package app exercises CSRF protection of a state-changing action that
// declares neither session nor sessionToken.
package app

import (
	"errors"
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
	signals datapages.Signals[struct {
		User string `json:"user"`
	}],
) (newSession datapages.NewSession[struct{}], err error) {
	return datapages.NewSession[struct{}]{UserID: signals.Values.User}, nil
}

// POSTDelete is /delete
//
// The dangerous action. It does not need to know who the visitor is.
// It therefore declares no session and is never checked.
func (p PageIndex) POSTDelete(
	_ *http.Request,
	signals datapages.Signals[struct {
		Confirm bool `json:"confirm"`
	}],
) error {
	if !signals.Values.Confirm {
		return nil
	}
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.deleted++
	return nil
}

// EventMailed is "mailed"
//
// [datapages.SubjectUser] marks the event as private.
type EventMailed struct {
	Recipient datapages.SubjectUser `json:"recipient"`
}

// PageInbox is /inbox
//
// [PageInbox.GET] has no session parameter. Its private event stream still
// reads the session. The document must render with that session to write the
// CSRF script for signed-in visitors.
type PageInbox struct{ App *App }

func (PageInbox) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">inbox</pre>`), nil
}

func (PageInbox) OnMailed(event EventMailed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(`<pre id="echo">mailed</pre>`))
}

// POSTMarkRead is /inbox/mark-read
func (PageInbox) POSTMarkRead(_ *http.Request) error { return nil }

// PageAbout is /about
//
// [PageAbout.GET] has no session parameter, and neither an app-wide Head nor
// an event stream requires a session. The document derives its CSRF script
// from the session cookie.
type PageAbout struct{ App *App }

func (PageAbout) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">about</pre>`), nil
}

// POSTPreview is /about/preview
//
// It returns a document without a session parameter.
func (PageAbout) POSTPreview(_ *http.Request) (
	body datapages.Component, err error,
) {
	return templ.Raw(`<pre id="echo">preview</pre>`), nil
}

// POSTPing is /ping
//
// Every page can call this app-level action.
func (*App) POSTPing(_ *http.Request) error { return nil }

// PageError404 is /not-found
//
// The 404 page reads the session: its document has to carry the CSRF script,
// or an action reachable from it is refused.
type PageError404 struct{ App *App }

func (PageError404) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">404 user=%s</pre>`, session.UserID(),
	)), nil
}

// PageError500 is /server-error
//
// The 500 page reads the session for the same reason.
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">500 user=%s</pre>`, session.UserID(),
	)), nil
}

// PageBoom is /boom
//
// GET fails, which is what the 500 page is rendered for.
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errBoom
}

var errBoom = errors.New("boom")
