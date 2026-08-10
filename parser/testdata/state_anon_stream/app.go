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

// TabState is the per-instance state of PageIndex.
type TabState struct {
	Counter int
}

// EventPostArchived is "posts.archived"
type EventPostArchived struct{}

// EventMessageSent is "message.sent"
type EventMessageSent struct {
	SubjectUser []string
}

// PageIndex is /
//
// The page serves signed-in and signed-out clients. Both hold per-tab
// state, and only the signed-in ones receive the private event.
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

func (PageIndex) OnPostArchived(
	event EventPostArchived,
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
