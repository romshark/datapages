package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventPing is "ping"
type EventPing struct{}

// TabContext is the per-instance state of every page that embeds Base.
type TabContext struct {
	Counter int
}

// Base is an abstract page. Both pages below inherit its stateful handlers,
// which binds them to the same state type.
type Base struct{ App *App }

func (Base) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *TabContext,
) error {
	return nil
}

func (Base) OnPing(
	event EventPing,
	sse *datastar.ServerSentEventGenerator,
	state *TabContext,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// PageOther is /other
type PageOther struct {
	App *App
	Base
}

func (PageOther) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}
