// Package app is an application under acceptance test. Its generated server
// is built and run, and the assertions live in ../acceptance_test.go.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
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
	SubjectStateID string
}

func (*App) Head(_ *http.Request) templ.Component {
	return templ.Raw(`<title>acceptance</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body templ.Component, err error) {
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
// The action writes per-tab state and patches nothing. Only the event
// handler can change what a tab shows, which makes delivery observable.
func (p PageIndex) POSTUpdate(
	r *http.Request,
	state *StateFilters,
	stateID string,
	signals struct {
		Filter string `json:"filter"`
	},
	dispatch func(EventFiltersUpdated) error,
) error {
	state.Filter = signals.Filter
	return dispatch(EventFiltersUpdated{SubjectStateID: stateID})
}

func (p PageIndex) OnFiltersUpdated(
	event EventFiltersUpdated,
	sse *datastar.ServerSentEventGenerator,
	state *StateFilters,
) error {
	state.Deliveries++
	return sse.PatchElementTempl(templ.Raw(status(state)))
}

func status(state *StateFilters) string {
	return fmt.Sprintf(
		`<div id="status">deliveries:%d filter:%s</div>`,
		state.Deliveries, templ.EscapeString(state.Filter))
}
