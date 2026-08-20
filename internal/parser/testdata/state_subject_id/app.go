package app

import (
	"net/http"

	"github.com/romshark/datapages"
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
	SubjectStateID datapages.SubjectStateID
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state *TabState,
) error {
	return nil
}

func (PageIndex) OnFiltersUpdated(
	event EventFiltersUpdated,
	sse datapages.SSE,
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
	signals datapages.Signals[struct {
		Filter string `json:"filter"`
	}],
	dispatch datapages.Dispatcher[EventFiltersUpdated],
) error {
	state.Filter = signals.Values.Filter
	return dispatch.Dispatch(EventFiltersUpdated{
		SubjectStateID: datapages.SubjectStateID(stateID),
	})
}
