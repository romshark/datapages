// Package app is the application the datapagestest package is tested against.
// Its handlers answer with each kind of response the package reads: patches whose root
// is a table row or body, a signal patch that deletes a signal, the three forms of a
// redirect, the signals of a DELETE, per-tab state, and events addressed by a signal
// and to a user.
package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

// App keeps the rows of the table the index page shows.
type App struct {
	mu   sync.Mutex
	rows []row
	next int
}

type row struct {
	ID    string
	Title string
}

// Session is the app's session type. PageRoom's GET takes one, which gives the room
// a second, anonymous stream for signed-out visitors.
type Session = datapages.Session[struct{}]

func (a *App) Head(_ *http.Request) datapages.Head {
	return templ.Raw(`<title>datapagestest</title>`)
}

func (a *App) add(title string) row {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	r := row{ID: strconv.Itoa(a.next), Title: title}
	a.rows = append(a.rows, r)
	return r
}

func (a *App) remove(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, r := range a.rows {
		if r.ID == id {
			a.rows = append(a.rows[:i], a.rows[i+1:]...)
			return true
		}
	}
	return false
}

// rowsHTML renders the rows whose title contains term.
func (a *App) rowsHTML(term string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var b strings.Builder
	for _, r := range a.rows {
		if strings.Contains(r.Title, term) {
			b.WriteString(rowHTML(r))
		}
	}
	return b.String()
}

func rowHTML(r row) string {
	return `<tr id="row-` + r.ID + `"><td>` + templ.EscapeString(r.Title) + `</td></tr>`
}

// EventRowAdded is "rows.added"
type EventRowAdded struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// EventRoomSaid is "room.said"
//
// Addressed by the room signal a stream connects with.
type EventRoomSaid struct {
	Room datapages.Subject `signal:"room"`

	Text string `json:"text"`
}

// EventNoticed is "noticed"
//
// Addressed to one user, and never on an anonymous stream.
type EventNoticed struct {
	Recipient datapages.SubjectUser

	Text string `json:"text"`
}

// StateIndex is the per-tab state of PageIndex: the term the tab filters rows by.
type StateIndex struct{ Term string }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Term string `query:"term" reflectsignal:"term"`
	}],
	_ datapages.Signals[struct {
		Term string `json:"term"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(`<div id="index" data-signals:draft="'it\'s'" data-signals:count="0">` +
		`<table><tbody id="rows">` + p.App.rowsHTML(query.Values.Term) +
		`</tbody></table></div>`), nil
}

func (PageIndex) StreamOpen(
	_ *http.Request,
	state datapages.State[StateIndex],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) error {
	state.Values.Term = signals.Values.Term
	return nil
}

func (PageIndex) OnRowAdded(
	event EventRowAdded,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	if !strings.Contains(event.Title, state.Values.Term) {
		return nil
	}
	return sse.PatchElementAt(
		templ.Raw(rowHTML(row(event))),
		"#rows", datapages.PatchModeAppend,
	)
}

// POSTAdd is /add
func (p PageIndex) POSTAdd(
	_ *http.Request,
	signals datapages.Signals[struct {
		Title string `json:"title"`
	}],
	rowAdded datapages.Dispatcher[EventRowAdded],
) error {
	if signals.Values.Title == "" {
		return fmt.Errorf("%w: no title", datapages.ErrBadRequest)
	}
	r := p.App.add(signals.Values.Title)
	return rowAdded.Dispatch(EventRowAdded(r))
}

// POSTFilter is /filter
//
// Answers with a patch whose root is the table body.
func (p PageIndex) POSTFilter(
	_ *http.Request,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) error {
	state.Values.Term = signals.Values.Term
	return sse.PatchElement(templ.Raw(
		`<tbody id="rows">` + p.App.rowsHTML(state.Values.Term) + `</tbody>`,
	))
}

// POSTClear is /clear
//
// Deletes the draft signal and resets the count.
func (PageIndex) POSTClear(_ *http.Request, sse datapages.SSE) error {
	return sse.PatchSignals(json.RawMessage(`{"draft":null,"count":0}`))
}

// POSTFail is /fail
//
// Fails after the event stream started, which leaves no status to send.
func (PageIndex) POSTFail(_ *http.Request, _ datapages.SSE) error {
	return fmt.Errorf("%w: always", datapages.ErrForbidden)
}

// DELETERow is /row/{id}
func (p PageIndex) DELETERow(
	_ *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Confirm bool `json:"confirm"`
	}],
) error {
	if !signals.Values.Confirm {
		return fmt.Errorf("%w: not confirmed", datapages.ErrBadRequest)
	}
	if !p.App.remove(path.Values.ID) {
		return datapages.ErrNotFound
	}
	return nil
}

// POSTGo is /go
//
// Redirects before any event stream starts.
func (PageIndex) POSTGo(_ *http.Request) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: "/done/"}, nil
}

// POSTGoStream is /go-stream
//
// Redirects through the event stream.
func (PageIndex) POSTGoStream(_ *http.Request, sse datapages.SSE) error {
	return sse.Redirect("/done/")
}

// PageDone is /done
type PageDone struct{ App *App }

func (PageDone) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<p id="done">done</p>`), nil
}

// PageOld is /old
//
// Moved to /done/, which a page load learns from an HTTP redirect.
type PageOld struct{ App *App }

func (PageOld) GET(_ *http.Request) (
	body datapages.Component, redirect datapages.Redirect, err error,
) {
	return nil, datapages.Redirect{URL: "/done/"}, nil
}

// PageRoom is /room
//
// Handles an event addressed by a signal and one addressed to a user.
type PageRoom struct{ App *App }

func (PageRoom) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	who := "guest"
	if !session.IsGuest() {
		who = session.UserID()
	}
	return templ.Raw(`<p id="said">` + templ.EscapeString(who) + `</p>`), nil
}

func (PageRoom) OnRoomSaid(event EventRoomSaid, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(`<p id="said">` +
		templ.EscapeString(string(event.Room)+": "+event.Text) + `</p>`))
}

func (PageRoom) OnNoticed(event EventNoticed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(`<p id="said">notice: ` +
		templ.EscapeString(event.Text) + `</p>`))
}

// POSTSay is /room/say
func (PageRoom) POSTSay(
	_ *http.Request,
	signals datapages.Signals[struct {
		Room string `json:"room"`
		Text string `json:"text"`
	}],
	roomSaid datapages.Dispatcher[EventRoomSaid],
) error {
	return roomSaid.Dispatch(EventRoomSaid{
		Room: datapages.Subject(signals.Values.Room),
		Text: signals.Values.Text,
	})
}

// POSTNotify is /room/notify
func (PageRoom) POSTNotify(
	_ *http.Request,
	signals datapages.Signals[struct {
		User string `json:"user"`
		Text string `json:"text"`
	}],
	noticed datapages.Dispatcher[EventNoticed],
) error {
	return noticed.Dispatch(EventNoticed{
		Recipient: datapages.SubjectUser(signals.Values.User),
		Text:      signals.Values.Text,
	})
}

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<p id="login">login</p>`), nil
}

// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(
	_ *http.Request,
	signals datapages.Signals[struct {
		User string `json:"user"`
	}],
) (
	newSession datapages.NewSession[struct{}],
	redirect datapages.Redirect,
	err error,
) {
	if signals.Values.User == "" {
		return newSession, redirect, datapages.ErrBadRequest
	}
	return datapages.NewSession[struct{}]{UserID: signals.Values.User},
		datapages.Redirect{URL: "/room/"}, nil
}
