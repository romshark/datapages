// Package app exercises per-tab state:
// a page whose handlers hold a value per browser tab,
// and an event addressed at one tab.
package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// StateFilters is the per-tab state of PageIndex.
type StateFilters struct {
	Filter     string
	Deliveries int
}

// EventFiltersUpdated is "filters.updated"
//
// Tab-scoped: only the tab named by SubjectStateID receives it.
type EventFiltersUpdated struct {
	SubjectStateID datapages.SubjectStateID
}

func (*App) Head(_ *http.Request) datapages.Component {
	return templ.Raw(`<title>acceptance</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return templ.Raw(status(new(StateFilters))), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *StateFilters,
) error {
	return nil
}

// POSTUpdate is /update
//
// The action writes per-tab state and patches nothing.
// Only the event handler can change what a tab shows, which makes delivery observable.
func (p PageIndex) POSTUpdate(
	r *http.Request,
	state *StateFilters,
	stateID string,
	signals struct {
		Filter string `json:"filter"`
	},
	dispatch datapages.Dispatch[EventFiltersUpdated],
) error {
	state.Filter = signals.Filter
	return dispatch(EventFiltersUpdated{
		SubjectStateID: datapages.SubjectStateID(stateID),
	})
}

func (p PageIndex) OnFiltersUpdated(
	event EventFiltersUpdated,
	sse datapages.SSE,
	state *StateFilters,
) error {
	state.Deliveries++
	return sse.PatchElement(templ.Raw(status(state)))
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
	streamID uint64,
	state *StateFilters,
) error {
	return ErrStreamOpen
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
	streamID uint64,
	state *StateFilters,
) error {
	panic(ErrStreamClose)
}

func status(state *StateFilters) string {
	return fmt.Sprintf(
		`<div id="status">deliveries:%d filter:%s</div>`,
		state.Deliveries, templ.EscapeString(state.Filter),
	)
}
