package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

type Session struct {
	UserID   string
	IssuedAt time.Time
}

type TabState struct {
	Filter string
}

// EventFiltersUpdated is "filters.updated"
type EventFiltersUpdated struct {
	SubjectStateID string
}

// EventMessageSent is "message.sent"
type EventMessageSent struct {
	SubjectUser []string
}

// PageIndex is /
//
// The page subscribes per tab for one event and per user for the other.
// One subscription list cannot carry both, and the parser rejects this.
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
) error {
	return nil
}

func (PageIndex) OnMessageSent(
	event EventMessageSent,
	sse *datastar.ServerSentEventGenerator,
	state *TabState,
) error {
	return nil
}
