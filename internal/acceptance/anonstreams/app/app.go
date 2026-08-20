// Package app exercises the stream a page serves to a visitor with no session:
// one page that scopes its events by a signal, and one that holds per-tab state.
//
// A page whose events are partly private serves two streams. The one for
// signed-in visitors carries both kinds; the anonymous one carries what is public,
// and has to subscribe by the same signal values and
// hold the same per-tab state as the other.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// Session is the app's session type. A handler that takes one is what
// gives the pages below a second, anonymous stream route.
type Session = datapages.Session[struct{}]

func (a *App) Head(_ *http.Request) datapages.Head {
	return templ.Raw(`<title>anonstreams</title>`)
}

// EventNoticed is "noticed"
//
// datapages.SubjectUser makes it private:
// only the streams of the named users receive it, and an anonymous stream never does.
type EventNoticed struct {
	Recipient datapages.SubjectUser

	Text string `json:"text"`
}

// EventRoomPosted is "room.posted"
//
// One subject field bound to a signal. A stream supplies the value when it
// connects and receives only what is published for it.
type EventRoomPosted struct {
	Room datapages.Subject `signal:"room"`

	Text string `json:"text"`
}

// EventTicked is "ticked"
//
// Public: every stream of the page receives it.
type EventTicked struct {
	N int `json:"n"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.IsGuest() {
		return templ.Raw(`<div id="out">index</div>`), nil
	}
	return templ.Raw(`<div id="out">index ` +
		templ.EscapeString(session.UserID()) + `</div>`), nil
}

// PageRooms is /rooms
//
// Its events are one private and one signal-scoped, so an anonymous visitor
// gets a stream of its own that subscribes by the room signal.
type PageRooms struct{ App *App }

func (PageRooms) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="out">rooms</div>`), nil
}

func (p PageRooms) OnRoomPosted(
	event EventRoomPosted,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">room %s: %s</div>`,
		templ.EscapeString(string(event.Room)), templ.EscapeString(event.Text),
	)))
}

func (p PageRooms) OnNoticed(
	event EventNoticed,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// POSTPost is /rooms/post
func (p PageRooms) POSTPost(
	_ *http.Request,
	signals datapages.Signals[struct {
		Room string `json:"room"`
		Text string `json:"text"`
	}],
	roomPosted datapages.Dispatcher[EventRoomPosted],
) error {
	return roomPosted.Dispatch(EventRoomPosted{
		Room: datapages.Subject(signals.Values.Room),
		Text: signals.Values.Text,
	})
}

// POSTNotice is /rooms/notice
func (p PageRooms) POSTNotice(
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

// StateTab is the per-tab state of PageTabs.
type StateTab struct{ Count int }

// PageTabs is /tabs
//
// Stateful, and with one private and one public event,
// so an anonymous visitor holds per-tab state on a stream of its own.
type PageTabs struct{ App *App }

func (PageTabs) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="count">count 0</div>`), nil
}

func (PageTabs) StreamOpen(
	_ *http.Request, streamID datapages.StreamID, state *StateTab,
) error {
	return nil
}

func (p PageTabs) OnTicked(
	event EventTicked,
	sse datapages.SSE,
	state *StateTab,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="count">count %d</div>`, state.Count,
	)))
}

func (p PageTabs) OnNoticed(
	event EventNoticed,
	sse datapages.SSE,
	state *StateTab,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="count">notice %s</div>`, templ.EscapeString(event.Text),
	)))
}

// POSTBump is /tabs/bump
//
// Writes the calling tab's state and dispatches the public event,
// which makes every tab render its own count.
func (p PageTabs) POSTBump(
	_ *http.Request,
	state *StateTab,
	ticked datapages.Dispatcher[EventTicked],
) error {
	state.Count++
	return ticked.Dispatch(EventTicked{N: state.Count})
}
