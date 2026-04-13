package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// TabState holds per-tab filters.
type TabState struct {
	Filter string
}

// EventFiltersUpdated is "filters.updated"
//
// Tab-scoped event: the server subscribes per-tab using the validated
// Datapages-Instance header at stream connect.
type EventFiltersUpdated struct {
	SubjectStateID string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

func (PageIndex) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *TabState,
) error {
	return nil
}

func (PageIndex) OnFiltersUpdated(
	event EventFiltersUpdated,
	sse *datastar.ServerSentEventGenerator,
	state *TabState,
	stateID string,
) error {
	_ = stateID
	return nil
}

// POSTUpdate is /update
func (PageIndex) POSTUpdate(
	r *http.Request,
	state *TabState,
	stateID string,
	signals struct {
		Filter string `json:"filter"`
	},
	dispatch func(EventFiltersUpdated) error,
) error {
	state.Filter = signals.Filter
	return dispatch(EventFiltersUpdated{SubjectStateID: stateID})
}
