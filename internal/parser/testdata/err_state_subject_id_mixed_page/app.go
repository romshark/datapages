package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

type TabState struct {
	Filter string
}

// EventFiltersUpdated is "filters.updated"
type EventFiltersUpdated struct {
	SubjectStateID datapages.SubjectStateID
}

// EventMessageSent is "message.sent"
type EventMessageSent struct {
	Recipient datapages.SubjectUser
}

// PageIndex is /
//
// The page subscribes per tab for one event and per user for the other.
// One subscription list cannot carry both, and the parser rejects this.
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	_ = session
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
	sse datapages.SSE,
	state *TabState,
) error {
	return nil
}

func (PageIndex) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
	state *TabState,
) error {
	return nil
}
