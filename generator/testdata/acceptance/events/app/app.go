// Package app exercises SSE streams: stream hooks, event dispatch, event
// handlers, subject scoping and handlers shared through an embedded type.
//
// Every subject used here is an exact one. The in-memory broker matches
// subjects as map keys. A subscription containing a NATS wildcard is never
// delivered to. See the bug_inmem_wildcard_subjects case.
package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
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

func echo(s string) templ.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// EventTick is "tick"
//
// No subject fields: every stream subscribed to it receives every one.
type EventTick struct {
	N int `json:"n"`
}

// EventRoomSaid is "room.said"
//
// One subject field bound to a signal. A stream supplies the signal when it
// connects and receives only what is published for that value.
type EventRoomSaid struct {
	SubjectRoom string `signal:"room"`

	Text string `json:"text"`
}

// EventRoomBroadcast is "room.broadcast"
//
// The plural form. The dispatched values are expanded into one subject each,
// so one dispatch reaches several rooms.
type EventRoomBroadcast struct {
	SubjectRoom []string `signal:"room"`

	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("index"), nil
}

// errStreamRefused is what StreamOpen returns when the application decides
// this stream must not run. A real one would fail here because a dependency
// is unavailable or the visitor may not watch this page.
var errStreamRefused = errors.New("this stream may not open")

func (p PageIndex) StreamOpen(r *http.Request, streamID uint64) error {
	if r.URL.Query().Get("refuse") != "" {
		return errStreamRefused
	}
	p.App.record("open(%d)", streamID)
	return nil
}

func (p PageIndex) StreamClose(_ *http.Request, streamID uint64) error {
	p.App.record("close(%d)", streamID)
	return nil
}

func (p PageIndex) OnTick(
	event EventTick,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	p.App.record("tick(%d,%d)", streamID, event.N)
	return sse.PatchElementTempl(
		templ.Raw(fmt.Sprintf(`<div id="out">tick %d</div>`, event.N)))
}

// POSTTick is /tick
func (p PageIndex) POSTTick(
	_ *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatch func(EventTick) error,
) error {
	return dispatch(EventTick{N: signals.N})
}

// PageLog is /log
type PageLog struct{ App *App }

func (p PageLog) GET(_ *http.Request) (body templ.Component, err error) {
	return echo(p.App.entries()), nil
}

// Notifier is an abstract type: a handler written once and embedded by the
// pages that want it. It is not a page and has no route.
type Notifier struct{ App *App }

func (n Notifier) OnTick(
	event EventTick,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(fmt.Sprintf(`<div id="shared">shared %d</div>`, event.N)))
}

// PageOther is /other
//
// It writes no handler of its own and receives events all the same.
type PageOther struct {
	App *App
	Notifier
}

func (PageOther) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("other"), nil
}

// PageRoom is /room
type PageRoom struct{ App *App }

func (PageRoom) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("room"), nil
}

func (p PageRoom) OnRoomSaid(
	event EventRoomSaid,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(`<div id="said">` + event.Text + `</div>`))
}

func (p PageRoom) OnRoomBroadcast(
	event EventRoomBroadcast,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(`<div id="broadcast">` + event.Text + `</div>`))
}

// POSTSay is /room/say
func (p PageRoom) POSTSay(
	_ *http.Request,
	signals struct {
		Room string `json:"room"`
		Text string `json:"text"`
	},
	dispatch func(EventRoomSaid) error,
) error {
	return dispatch(EventRoomSaid{SubjectRoom: signals.Room, Text: signals.Text})
}

// POSTBroadcast is /room/broadcast
func (p PageRoom) POSTBroadcast(
	_ *http.Request,
	signals struct {
		Rooms []string `json:"rooms"`
		Text  string   `json:"text"`
	},
	dispatch func(EventRoomBroadcast) error,
) error {
	return dispatch(EventRoomBroadcast{
		SubjectRoom: signals.Rooms,
		Text:        signals.Text,
	})
}
