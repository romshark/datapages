package app

import (
	"net/http"

	"datapagestest/fixture/err_event_shared/eventsa"
	"datapagestest/fixture/err_event_shared/eventsb"

	"github.com/romshark/datapages"
)

type App struct{}

// EventLocalDup is "a.dup"
//
// The subject is the one eventsa.EventDup claims.
// The duplicate check spans every event of the application, wherever it is declared.
type EventLocalDup struct {
	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIndex) OnLocalDup(event EventLocalDup, sse datapages.SSE) error {
	return nil
}

func (PageIndex) OnDupA(event eventsa.EventDup, sse datapages.SSE) error {
	return nil
}

// OnDupB handles an event of a second package whose type name is taken.
func (PageIndex) OnDupB(event eventsb.EventDup, sse datapages.SSE) error {
	return nil
}

// OnNoComment handles a foreign type that declares no subject.
func (PageIndex) OnNoComment(event eventsb.EventNoComment, sse datapages.SSE) error {
	return nil
}
