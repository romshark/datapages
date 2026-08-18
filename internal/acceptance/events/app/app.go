// Package app exercises SSE streams: stream hooks, event dispatch, event handlers,
// subject scoping and handlers shared through an embedded type.
//
// Every subject used here is an exact one. Subjects a page subscribes to by
// pattern are covered by the wildcardsubjects case.
package app

import (
	"context"
	"errors"
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

// EventTick is "tick"
//
// No subject fields: every stream subscribed to it receives every one.
type EventTick struct {
	N int `json:"n"`
}

// EventPong is "pong"
//
// The second event type of an action that dispatches two.
type EventPong struct {
	N int `json:"n"`
}

// EventStreamGone is "stream.gone"
//
// Dispatched from StreamClose, which runs while the stream is being torn down.
// The publish uses the request context without its cancelation,
// otherwise the event would never leave the server.
type EventStreamGone struct {
	StreamID uint64 `json:"stream_id"`
}

// EventRoomSaid is "room.said"
//
// One subject field bound to a signal. A stream supplies the signal when it
// connects and receives only what is published for that value.
type EventRoomSaid struct {
	Room datapages.Subject `signal:"room"`

	Text string `json:"text"`
}

// EventRoomBroadcast is "room.broadcast"
//
// One dispatch publishes to one subject, so reaching several rooms is a loop
// over the dispatcher, which leaves the handler in control of the failures.
type EventRoomBroadcast struct {
	Room datapages.Subject `signal:"room"`

	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
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

func (p PageIndex) StreamClose(
	_ *http.Request,
	streamID uint64,
	dispatchGone datapages.Dispatch[EventStreamGone],
) error {
	p.App.record("close(%d)", streamID)
	return dispatchGone(EventStreamGone{StreamID: streamID})
}

func (p PageIndex) OnStreamGone(event EventStreamGone, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="gone">gone %d</div>`, event.StreamID,
	)))
}

func (p PageIndex) OnPong(event EventPong, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(fmt.Sprintf(
		`<div id="pong">pong %d</div>`, event.N,
	)))
}

func (p PageIndex) OnTick(
	event EventTick,
	sse datapages.SSE,
	streamID uint64,
) error {
	p.App.record("tick(%d,%d)", streamID, event.N)
	return sse.PatchElement(
		templ.Raw(fmt.Sprintf(`<div id="out">tick %d</div>`, event.N)),
	)
}

// POSTTick is /tick
func (p PageIndex) POSTTick(
	_ *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatch datapages.Dispatch[EventTick],
) error {
	return dispatch(EventTick{N: signals.N})
}

// POSTBoth is /both
//
// Two dispatchers in one handler, one per event type.
func (p PageIndex) POSTBoth(
	_ *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatchTick datapages.Dispatch[EventTick],
	dispatchPong datapages.Dispatch[EventPong],
) error {
	return errors.Join(
		dispatchTick(EventTick{N: signals.N}),
		dispatchPong(EventPong{N: signals.N}),
	)
}

// POSTCanceled is /canceled
//
// Dispatches with a context that is already done.
// A broker that honors the context refuses the publish,
// which is how the test tells the option apart from the handler's own context.
func (p PageIndex) POSTCanceled(
	r *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatch datapages.Dispatch[EventTick],
) error {
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	return dispatch(EventTick{N: signals.N}, datapages.WithDispatchContext(ctx))
}

// PageLog is /log
type PageLog struct{ App *App }

func (p PageLog) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo(p.App.entries()), nil
}

// Notifier is an abstract type: a handler written once and embedded by the
// pages that want it. It is not a page and has no route.
type Notifier struct{ App *App }

func (n Notifier) OnTick(
	event EventTick,
	sse datapages.SSE,
) error {
	return sse.PatchElement(
		templ.Raw(fmt.Sprintf(`<div id="shared">shared %d</div>`, event.N)),
	)
}

// PageOther is /other
//
// It writes no handler of its own and receives events all the same.
type PageOther struct {
	App *App
	Notifier
}

func (PageOther) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("other"), nil
}

// PageRoom is /room
type PageRoom struct{ App *App }

func (PageRoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("room"), nil
}

func (p PageRoom) OnRoomSaid(
	event EventRoomSaid,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(`<div id="said">` + event.Text + `</div>`))
}

func (p PageRoom) OnRoomBroadcast(
	event EventRoomBroadcast,
	sse datapages.SSE,
) error {
	return sse.PatchElement(
		templ.Raw(`<div id="broadcast">` + event.Text + `</div>`),
	)
}

// POSTSay is /room/say
func (p PageRoom) POSTSay(
	_ *http.Request,
	signals struct {
		Room string `json:"room"`
		Text string `json:"text"`
	},
	dispatch datapages.Dispatch[EventRoomSaid],
) error {
	return dispatch(EventRoomSaid{
		Room: datapages.Subject(signals.Room), Text: signals.Text,
	})
}

// POSTBroadcast is /room/broadcast
func (p PageRoom) POSTBroadcast(
	_ *http.Request,
	signals struct {
		Rooms []string `json:"rooms"`
		Text  string   `json:"text"`
	},
	dispatch datapages.Dispatch[EventRoomBroadcast],
) error {
	for _, room := range signals.Rooms {
		err := dispatch(EventRoomBroadcast{
			Room: datapages.Subject(room),
			Text: signals.Text,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
