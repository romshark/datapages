package simple

import (
	"net/http"
	"sync/atomic"

	"github.com/romshark/datapages"
)

// EventCounterUpdated is "simple.counter.updated"
//
// The subject carries the name of the application: the two applications of
// this module must not claim one subject, since a broker given to both would
// carry each one's events to the other.
type EventCounterUpdated struct{}

type App struct{ counter atomic.Int32 }

func (*App) Head(_ *http.Request) datapages.Head { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageCounter(p.App.counter.Load()), nil
}

// POSTAdd is /add/{$}
func (p PageIndex) POSTAdd(
	r *http.Request, counterUpdated datapages.Dispatcher[EventCounterUpdated],
	query datapages.Query[struct {
		Delta int32 `query:"delta"`
	}],
) error {
	p.App.counter.Add(query.Values.Delta)
	return counterUpdated.Dispatch(EventCounterUpdated{})
}

func (p PageIndex) OnCounterUpdated(
	event EventCounterUpdated, sse datapages.SSE,
) error {
	return sse.PatchElement(pageCounter(p.App.counter.Load()))
}
