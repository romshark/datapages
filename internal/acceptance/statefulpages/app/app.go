// Package app exercises per-tab state:
// a page whose handlers hold a value per browser tab,
// and an event addressed at one tab.
package app

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	lockClosed sync.Mutex
	closed     []string
}

// recordClosed keeps what a StreamClose read from the state it was given.
// A close hook takes no sse and runs after its tab is gone,
// which leaves the app the only place it can report from.
func (a *App) recordClosed(filter string) {
	a.lockClosed.Lock()
	defer a.lockClosed.Unlock()
	a.closed = append(a.closed, filter)
}

// Closed returns what every StreamClose so far read, in the order they ran.
func (a *App) Closed() []string {
	a.lockClosed.Lock()
	defer a.lockClosed.Unlock()
	return slices.Clone(a.closed)
}

// StateFilters is the per-tab state of PageIndex.
type StateFilters struct {
	Filter     string
	Deliveries int
	Panic      bool
}

// EventFiltersUpdated is "filters.updated"
//
// Tab-scoped: only the tab named by SubjectStateID receives it.
type EventFiltersUpdated struct {
	SubjectStateID datapages.SubjectStateID
}

func (*App) Head(_ *http.Request) datapages.Head {
	return templ.Raw(`<title>acceptance</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return templ.Raw(status(new(StateFilters))), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[StateFilters],
) error {
	return nil
}

// POSTUpdate is /update
//
// The action writes per-tab state and patches nothing.
// Only the event handler can change what a tab shows, which makes delivery observable.
func (p PageIndex) POSTUpdate(
	r *http.Request,
	state datapages.State[StateFilters],
	stateID string,
	signals datapages.Signals[struct {
		Filter string `json:"filter"`
		Panic  bool   `json:"panic"`
	}],
	dispatch datapages.Dispatcher[EventFiltersUpdated],
) error {
	state.Values.Filter = signals.Values.Filter
	state.Values.Panic = signals.Values.Panic
	return dispatch.Dispatch(EventFiltersUpdated{
		SubjectStateID: datapages.SubjectStateID(stateID),
	})
}

// ErrEventHandler is what PageIndex.OnFiltersUpdated panics with.
var ErrEventHandler = errors.New("this event handler does not return quietly")

func (p PageIndex) OnFiltersUpdated(
	event EventFiltersUpdated,
	sse datapages.SSE,
	state datapages.State[StateFilters],
) error {
	if state.Values.Panic {
		panic(ErrEventHandler)
	}
	state.Values.Deliveries++
	return sse.PatchElement(templ.Raw(status(state.Values)))
}

// ErrStreamOpen is what PageFailOpen.StreamOpen answers with, every time.
var ErrStreamOpen = errors.New("this stream never opens")

// PageFailOpen is /failopen
//
// Its stream never opens. The instance is reserved before the open hook runs
// and the close hook that gives it back is wired up only after the hook succeeded,
// which leaves the failed open itself to release what it took.
type PageFailOpen struct{ App *App }

func (p PageFailOpen) GET(r *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="status">failopen</div>`), nil
}

func (p PageFailOpen) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[StateFilters],
) error {
	return ErrStreamOpen
}

// PageCloseState is /closestate
//
// Its StreamClose reads the state one last time. The value it finds there must
// be the one the tab's last handler wrote, not the zero value its stream started with.
type PageCloseState struct{ App *App }

func (PageCloseState) GET(r *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="status">closestate</div>`), nil
}

// POSTMark is /closestate/mark
func (PageCloseState) POSTMark(
	r *http.Request,
	state datapages.State[StateFilters],
	signals datapages.Signals[struct {
		Filter string `json:"filter"`
	}],
) error {
	state.Values.Filter = signals.Values.Filter
	return nil
}

func (p PageCloseState) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[StateFilters],
) error {
	p.App.recordClosed(state.Values.Filter)
	return nil
}

// ErrStreamClose is what PagePanicOnClose.StreamClose panics with.
var ErrStreamClose = errors.New("this stream does not close quietly")

// PagePanicOnClose is /panicclose
//
// Its StreamClose panics. That hook runs on the watchdog goroutine,
// outside the recovery of net/http, and it holds the slot mutex while it runs.
type PagePanicOnClose struct{ App *App }

func (p PagePanicOnClose) GET(r *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<div id="status">panicclose</div>`), nil
}

func (p PagePanicOnClose) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[StateFilters],
) error {
	panic(ErrStreamClose)
}

func status(state *StateFilters) string {
	return fmt.Sprintf(
		`<div id="status">deliveries:%d filter:%s</div>`,
		state.Deliveries, templ.EscapeString(state.Filter),
	)
}
