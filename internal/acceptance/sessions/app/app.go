// Package app exercises sessions: issuing one, reading one, closing one,
// the token that names it, and the events that are addressed to the user who owns it.
package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	mu  sync.Mutex
	log []string
}

func (a *App) record(format string, args ...any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.log = append(a.log, fmt.Sprintf(format, args...))
}

func (a *App) entries() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.log, " ")
}

func echo(s string) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// SessionData is what this application keeps in the session
// on top of what datapages keeps.
type SessionData struct {
	Nickname string `json:"nickname"`
}

// Session is the session type the generated server manages.
type Session = datapages.Session[SessionData]

// EventNotice is "notice"
//
// datapages.SubjectUser makes the event private.
// It is delivered to the streams of the named users only.
type EventNotice struct {
	Recipient datapages.SubjectUser

	Text string `json:"text"`
}

// EventBroadcast is "broadcast"
//
// A public event on the same page as a private one. The page then serves two
// kinds of stream: one for signed-in visitors, which also carries the private event,
// and an anonymous one, which must not.
type EventBroadcast struct {
	Text string `json:"text"`
}

// Head is the shared head. It is given the session and can therefore differ
// for a signed-in visitor.
func (a *App) Head(_ *http.Request, session Session) datapages.Component {
	if session.IsGuest() {
		return templ.Raw(`<title>anonymous</title>`)
	}
	return templ.Raw(`<title>` + session.UserID() + `</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.IsGuest() {
		return echo("anonymous"), nil
	}
	return echo(fmt.Sprintf("user=%s nickname=%s issued=%d",
		session.UserID(), session.Data().Nickname, session.IssuedAt().Unix())), nil
}

func (p PageIndex) OnNotice(
	event EventNotice,
	sse datapages.SSE,
	session Session,
) error {
	return sse.PatchElement(templ.Raw(
		`<div id="notice">` + session.UserID() + ": " + event.Text + `</div>`,
	))
}

func (p PageIndex) OnBroadcast(
	event EventBroadcast,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(
		`<div id="broadcast">` + event.Text + `</div>`,
	))
}

// PageToken is /token
//
// A handler may ask for the token rather than the session.
// An application needs the token to address a single one of a user's sessions.
type PageToken struct{ App *App }

func (PageToken) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.Token() == "" {
		return echo("no token"), nil
	}
	return echo("token of length " + fmt.Sprint(len(session.Token()))), nil
}

// PageSecret is /secret
//
// A page only a signed-in visitor may read.
// The status comes from the sentinel the handler returns.
type PageSecret struct{ App *App }

func (PageSecret) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.IsGuest() {
		return nil, datapages.ErrForbidden
	}
	return echo("secret for " + session.UserID()), nil
}

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("login"), nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	_ *http.Request,
	signals struct {
		User     string `json:"user"`
		Nickname string `json:"nickname"`
	},
) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	if signals.User == "" {
		return newSession, redirect, datapages.ErrBadRequest
	}
	p.App.record("login(%s)", signals.User)
	return datapages.NewSession[SessionData]{
		UserID: signals.User,
		Data:   SessionData{Nickname: signals.Nickname},
	}, datapages.Redirect{URL: "/"}, nil
}

// POSTNotify is /login/notify
//
// Dispatches a private event to one user.
func (p PageLogin) POSTNotify(
	_ *http.Request,
	signals struct {
		User string `json:"user"`
		Text string `json:"text"`
	},
	dispatch datapages.Dispatch[EventNotice],
) error {
	return dispatch(EventNotice{
		Recipient: datapages.SubjectUser(signals.User),
		Text:      signals.Text,
	})
}

// POSTBroadcast is /login/broadcast
//
// Dispatches the public event. Anyone watching the page receives it.
func (p PageLogin) POSTBroadcast(
	_ *http.Request,
	signals struct {
		Text string `json:"text"`
	},
	dispatch datapages.Dispatch[EventBroadcast],
) error {
	return dispatch(EventBroadcast{Text: signals.Text})
}

// POSTRename is /login/rename
//
// An action that reads the session.
func (p PageLogin) POSTRename(
	_ *http.Request,
	session Session,
	signals struct {
		Nickname string `json:"nickname"`
	},
) error {
	p.App.record("rename(%s,%s)", session.UserID(), signals.Nickname)
	return nil
}

// POSTSignOut is /sign-out
func (a *App) POSTSignOut(_ *http.Request, session Session) (
	closeSession bool, redirect datapages.Redirect, err error,
) {
	a.record("signout(%s)", session.UserID())
	return true, datapages.Redirect{URL: "/"}, nil
}

// PageLog is /log
type PageLog struct{ App *App }

func (p PageLog) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo(p.App.entries()), nil
}
