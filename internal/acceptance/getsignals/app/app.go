// Package app exercises what a page GET may take besides the request and what it
// may return besides its body: signals sent by the client, and a session it issues.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// Session is the app's session type.
type Session = datapages.Session[struct{}]

// PageIndex is /
//
// GET reads signals and the session, and echoes both.
type PageIndex struct{ App *App }

func (PageIndex) GET(
	_ *http.Request,
	session Session,
	signals datapages.Signals[struct {
		Term string `json:"term"`
		Page int    `json:"page"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">term=%s page=%d user=%s</pre>`,
		templ.EscapeString(signals.Values.Term), signals.Values.Page,
		templ.EscapeString(session.UserID()),
	)), nil
}

// PageNested is /nested
//
// A signals struct that nests. A nested struct is a nested signal: the client
// sends {"foo":{"bar":{"bazz":"x"},"fuzz":"y"}} for the signals foo.bar.bazz
// and foo.fuzz, and a query parameter reflects one of them by that path.
type PageNested struct{ App *App }

func (PageNested) GET(
	_ *http.Request,
	signals datapages.Signals[struct {
		Foo struct {
			Bar struct {
				Bazz string `json:"bazz"`
			} `json:"bar"`
			Fuzz string `json:"fuzz"`
		} `json:"foo"`
	}],
	query datapages.Query[struct {
		Fuzz string `query:"fuzz" reflectsignal:"foo.fuzz"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(
		`<pre id="echo">bazz=%s fuzz=%s query=%s</pre>`,
		templ.EscapeString(signals.Values.Foo.Bar.Bazz),
		templ.EscapeString(signals.Values.Foo.Fuzz),
		templ.EscapeString(query.Values.Fuzz),
	)), nil
}

// PageEnter is /enter
//
// GET issues a session on a page load.
// The cookie has to be set before the page is written.
type PageEnter struct{ App *App }

func (PageEnter) GET(_ *http.Request) (
	body datapages.Component,
	newSession datapages.NewSession[struct{}],
	err error,
) {
	return templ.Raw(`<pre id="echo">entered</pre>`),
		datapages.NewSession[struct{}]{UserID: "alice"}, nil
}

// POSTLeave is /leave
//
// An action that ends the session the caller holds.
func (PageIndex) POSTLeave(_ *http.Request, session Session) (
	closeSession datapages.CloseSession,
	err error,
) {
	return session.Token() != "", nil
}
