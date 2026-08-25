// Package frontend is the session-carrying application of a module that builds
// two of them.
//
// Its server is generated with SessionData and datapages.DisablePrometheus.
// Admin is generated with the opposite of both. Neither type argument is a
// property of the module: each app package is read on its own and generated
// for what its own calls name.
package frontend

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// SessionData is what this application keeps in the session
// on top of what datapages keeps.
type SessionData struct {
	Nickname string `json:"nickname"`
}

// Session is the session type the generated server manages.
type Session = datapages.Session[SessionData]

// EventNotice is "notice"
type EventNotice struct {
	Text string `json:"text"`
}

func echo(s string) datapages.Component {
	return templ.Raw(`<pre id="echo">` + s + `</pre>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.IsGuest() {
		return echo("anonymous"), nil
	}
	return echo("user=" + session.UserID() +
		" nickname=" + session.Data().Nickname), nil
}

func (PageIndex) OnNotice(
	event EventNotice,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(
		`<div id="out">` + event.Text + `</div>`,
	))
}

// POSTSignIn is /sign-in
func (PageIndex) POSTSignIn(
	_ *http.Request,
	signals datapages.Signals[struct {
		User     string `json:"user"`
		Nickname string `json:"nickname"`
	}],
) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	if signals.Values.User == "" {
		return newSession, redirect, datapages.ErrBadRequest
	}
	return datapages.NewSession[SessionData]{
		UserID: signals.Values.User,
		Data:   SessionData{Nickname: signals.Values.Nickname},
	}, datapages.Redirect{URL: "/"}, nil
}

// POSTNotice is /notice
func (PageIndex) POSTNotice(
	_ *http.Request,
	signals datapages.Signals[struct {
		Text string `json:"text"`
	}],
	notice datapages.Dispatcher[EventNotice],
) error {
	return notice.Dispatch(EventNotice{Text: signals.Values.Text})
}
