package app

import (
	"net/http"

	"datapagestest/fixture/event_shared/events"

	"github.com/romshark/datapages"
)

type App struct{}

// EventLocal is "local"
type EventLocal struct {
	N int `json:"n"`
}

// POSTAppPublish is /app-publish
//
// An action of the application itself,
// which dispatches from the other writer of the generated code.
func (*App) POSTAppPublish(
	r *http.Request,
	shared datapages.Dispatcher[events.EventShared],
) error {
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIndex) OnLocal(event EventLocal, sse datapages.SSE) error {
	return nil
}

// OnShared handles an event this application takes part in without declaring it.
func (PageIndex) OnShared(event events.EventShared, sse datapages.SSE) error {
	return nil
}

// OnRoom handles a foreign event with a subject field,
// which the stream subscribes to by prefix.
func (PageIndex) OnRoom(event events.EventRoom, sse datapages.SSE) error {
	return nil
}

// OnCalc handles a foreign event whose subject segment comes from a signal.
func (PageIndex) OnCalc(event events.EventCalc, sse datapages.SSE) error {
	return nil
}

// POSTPublish is /publish
func (PageIndex) POSTPublish(
	r *http.Request,
	local datapages.Dispatcher[EventLocal],
	shared datapages.Dispatcher[events.EventShared],
	room datapages.Dispatcher[events.EventRoom],
	calc datapages.Dispatcher[events.EventCalc],
) error {
	return nil
}
