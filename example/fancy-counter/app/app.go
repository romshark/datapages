package app

import (
	"net/http"
	"sync/atomic"

	"github.com/romshark/datapages"
)

// EventCounterUpdated is "counter.updated"
type EventCounterUpdated struct{}

type App struct{ counter atomic.Int32 }

func (*App) Head(_ *http.Request) datapages.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageCounter(p.App.counter.Load()), nil
}

// POSTAdd is /add/{$}
func (p PageIndex) POSTAdd(
	r *http.Request, counterUpdated datapages.Dispatcher[EventCounterUpdated],
	query struct {
		Delta int32 `query:"delta"`
	},
) error {
	p.App.counter.Add(query.Delta)
	return counterUpdated.Dispatch(EventCounterUpdated{})
}

// POSTSet is /set/{value}/{$}
func (p PageIndex) POSTSet(
	r *http.Request, counterUpdated datapages.Dispatcher[EventCounterUpdated],
	path struct {
		Value int32 `path:"value"`
	},
	signals struct {
		SetValue int32 `json:"setvalue"`
	},
) error {
	v := signals.SetValue
	if path.Value != 0 {
		v = path.Value
	}
	p.App.counter.Store(v)
	return counterUpdated.Dispatch(EventCounterUpdated{})
}

func (p PageIndex) OnCounterUpdated(
	event EventCounterUpdated, sse datapages.SSE,
) error {
	return sse.PatchElement(counterValue(p.App.counter.Load()))
}
