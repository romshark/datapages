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
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	lock sync.Mutex
	// opened and closed count the streams of PagePanic.
	opened, closed int
	recovered      []error
}

// PanicStreams reports how many streams of PagePanic ran StreamOpen and StreamClose.
func (a *App) PanicStreams() (opened, closed int) {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.opened, a.closed
}

// Recovered returns the errors RecoverError received.
func (a *App) Recovered() []error {
	a.lock.Lock()
	defer a.lock.Unlock()
	return append([]error(nil), a.recovered...)
}

// RecoverError records the error and writes it into the page.
func (a *App) RecoverError(err error, sse datapages.SSE) error {
	a.lock.Lock()
	a.recovered = append(a.recovered, err)
	a.lock.Unlock()
	return sse.PatchElement(templ.Raw(`<div id="out">recovered: ` +
		templ.EscapeString(err.Error()) + `</div>`))
}

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

// EventDMed is "dmed"
//
// Two subject fields on one declaration line. Each names a segment,
// which keeps the event private and out of every anonymous stream.
type EventDMed struct {
	To, Cc datapages.SubjectUser

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

func (p PageRooms) OnDMed(event EventDMed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">dm: %s</div>`, templ.EscapeString(event.Text),
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

// POSTDM is /rooms/dm
func (p PageRooms) POSTDM(
	_ *http.Request,
	signals datapages.Signals[struct {
		To   string `json:"to"`
		Cc   string `json:"cc"`
		Text string `json:"text"`
	}],
	dmed datapages.Dispatcher[EventDMed],
) error {
	return dmed.Dispatch(EventDMed{
		To:   datapages.SubjectUser(signals.Values.To),
		Cc:   datapages.SubjectUser(signals.Values.Cc),
		Text: signals.Values.Text,
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
	_ *http.Request, streamID datapages.StreamID, state datapages.State[StateTab],
) error {
	return nil
}

func (p PageTabs) OnTicked(
	event EventTicked,
	sse datapages.SSE,
	state datapages.State[StateTab],
) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="count">count %d</div>`, state.Values.Count,
	)))
}

func (p PageTabs) OnNoticed(
	event EventNoticed,
	sse datapages.SSE,
	state datapages.State[StateTab],
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
	state datapages.State[StateTab],
	ticked datapages.Dispatcher[EventTicked],
) error {
	state.Values.Count++
	return ticked.Dispatch(EventTicked{N: state.Values.Count})
}

// PagePost is /post/{slug}
//
// The redirect to its anonymous stream is built from the request path,
// which arrives decoded: a slug carrying "?" or "#" has to survive it.
type PagePost struct{ App *App }

func (PagePost) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
	session Session,
) (body datapages.Component, err error) {
	_ = session
	return templ.Raw(`<div id="out">post ` +
		templ.EscapeString(path.Values.Slug) + `</div>`), nil
}

func (p PagePost) OnTicked(event EventTicked, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">tick %d</div>`, event.N,
	)))
}

func (p PagePost) OnNoticed(event EventNoticed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// PageBackground is /background
//
// Mixed like PageRooms, and its GET keeps the streams open while the tab is hidden.
// A signed-in visitor and a guest connect to different streams,
// and both have to stay open.
type PageBackground struct{ App *App }

func (PageBackground) GET(_ *http.Request) (
	body datapages.Component,
	bgStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return templ.Raw(`<div id="out">background</div>`), true, nil
}

func (p PageBackground) OnTicked(event EventTicked, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">tick %d</div>`, event.N,
	)))
}

func (p PageBackground) OnNoticed(event EventNoticed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// PageBackgroundPost is /background-post/{slug}
//
// PageBackground with a path variable, which the stream path is built from.
type PageBackgroundPost struct{ App *App }

func (PageBackgroundPost) GET(
	_ *http.Request,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
) (
	body datapages.Component,
	bgStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return templ.Raw(`<div id="out">background post ` +
		templ.EscapeString(path.Values.Slug) + `</div>`), true, nil
}

func (p PageBackgroundPost) OnTicked(event EventTicked, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">tick %d</div>`, event.N,
	)))
}

func (p PageBackgroundPost) OnNoticed(event EventNoticed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// EventFaulted is "faulted"
//
// Public: the anonymous stream of PagePanic receives it.
type EventFaulted struct{}

// PagePanic is /panic
//
// Mixed like PageRooms, which gives it an anonymous stream.
// Its public event handler panics, the way a handler fails on data it does not expect.
type PagePanic struct{ App *App }

func (PagePanic) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="out">panic</div>`), nil
}

func (p PagePanic) StreamOpen(_ *http.Request, _ datapages.StreamID) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.opened++
	return nil
}

func (p PagePanic) StreamClose(_ *http.Request, _ datapages.StreamID) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.closed++
	return nil
}

func (PagePanic) OnFaulted(event EventFaulted, sse datapages.SSE) error {
	panic("the faulted handler panicked")
}

func (PagePanic) OnNoticed(event EventNoticed, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="out">notice: %s</div>`, templ.EscapeString(event.Text),
	)))
}

// POSTFault is /panic/fault
func (PagePanic) POSTFault(
	_ *http.Request, faulted datapages.Dispatcher[EventFaulted],
) error {
	return faulted.Dispatch(EventFaulted{})
}
