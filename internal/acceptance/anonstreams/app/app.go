// Package app exercises the stream a page serves to a visitor with no session.
//
// A page whose events are partly private serves two streams. The one for
// signed-in visitors carries both kinds; the anonymous one carries what is
// public, and has to subscribe by the same signal values as the other.
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

func (a *App) Head(_ *http.Request) datapages.Component {
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
	signals struct {
		Room string `json:"room"`
		Text string `json:"text"`
	},
	dispatch datapages.Dispatch[EventRoomPosted],
) error {
	return dispatch(EventRoomPosted{Room: datapages.Subject(signals.Room), Text: signals.Text})
}

// POSTNotice is /rooms/notice
func (p PageRooms) POSTNotice(
	_ *http.Request,
	signals struct {
		User string `json:"user"`
		Text string `json:"text"`
	},
	dispatch datapages.Dispatch[EventNoticed],
) error {
	return dispatch(EventNoticed{
		Recipient: datapages.SubjectUser(signals.User),
		Text:      signals.Text,
	})
}

// PageFeed is /feed
//
// One private event and one public, and no subject bound to a signal,
// so its stream connects with nothing to supply.
type PageFeed struct{ App *App }

func (PageFeed) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="feed">feed</div>`), nil
}

func (p PageFeed) OnTicked(
	event EventTicked,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="feed">tick %d</div>`, event.N,
	)))
}

func (p PageFeed) OnNoticed(
	event EventNoticed,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="feed">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// POSTTick is /feed/tick
func (p PageFeed) POSTTick(
	_ *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatch datapages.Dispatch[EventTicked],
) error {
	return dispatch(EventTicked{N: signals.N})
}
