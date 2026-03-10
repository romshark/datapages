package app

import (
	"net/http"
	"sync/atomic"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// EventCounterUpdated is "counter.updated"
type EventCounterUpdated struct{}

type App struct{ counter atomic.Int32 }

func (*App) Head(_ *http.Request) templ.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return pageCounter(p.App.counter.Load()), nil
}

// POSTAdd is /add/{$}
func (p PageIndex) POSTAdd(
	r *http.Request, dispatch func(EventCounterUpdated) error,
	query struct {
		Delta int32 `query:"delta"`
	},
) error {
	p.App.counter.Add(query.Delta)
	return dispatch(EventCounterUpdated{})
}

// POSTSet is /set/{value}/{$}
func (p PageIndex) POSTSet(
	r *http.Request, dispatch func(EventCounterUpdated) error,
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
	return dispatch(EventCounterUpdated{})
}

func (p PageIndex) OnCounterUpdated(
	event EventCounterUpdated, sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(counterValue(p.App.counter.Load()))
}
