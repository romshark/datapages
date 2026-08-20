package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// TabState is the per-instance state of PageIndex.
type TabState struct {
	Counter int
}

// EventPostArchived is "posts.archived"
type EventPostArchived struct{}

// EventMessageSent is "message.sent"
type EventMessageSent struct {
	Recipient datapages.SubjectUser
}

// PageIndex is /
//
// The page serves signed-in and signed-out clients. Both hold per-tab
// state, and only the signed-in ones receive the private event.
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	_ = session
	return nil, nil
}

func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state *TabState,
) error {
	return nil
}

func (PageIndex) OnPostArchived(
	event EventPostArchived,
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
