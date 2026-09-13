package app

import (
	"net/http"

	"datapagestest/fixture/event_shared_alias/mid"

	"github.com/romshark/datapages"
)

type App struct{}

// EventOther is the event of the deep package under a name of this one.
// An alias is no declaration: the subject and the payload are read where the
// type behind it is written.
type EventOther = mid.EventOther

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// OnDeep handles an event reached through an alias of an imported package.
func (PageIndex) OnDeep(event mid.EventDeep, sse datapages.SSE) error {
	return nil
}

// OnOther handles an event reached through an alias of the app package.
func (PageIndex) OnOther(event EventOther, sse datapages.SSE) error {
	return nil
}

// POSTPublish is /publish
func (PageIndex) POSTPublish(
	r *http.Request,
	deep datapages.Dispatcher[mid.EventDeep],
	other datapages.Dispatcher[EventOther],
) error {
	return nil
}
