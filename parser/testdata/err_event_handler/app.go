//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// EventFoo is "foo"
type EventFoo struct {
	Foo string `json:"foo"`
}

// EventBar is "bar"
type EventBar struct {
	Bar string `json:"bar"`
}

// EventBazz is "bazz"
type EventBazz struct {
	Bazz string `json:"bazz"`
}

// EventFuzz is "fuzz"
type EventFuzz struct {
	Fuzz string `json:"fuzz"`
}

func (PageIndex) OnEvent(
	event EventFoo,
	sse *datastar.ServerSentEventGenerator,
) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
}

func (PageIndex) OnEventBar(
	event EventBar,
	sse *datastar.ServerSentEventGenerator,
) (int, error) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return 0, nil
}

func (PageIndex) OnEventBazz(
	event EventBazz,
	sse *datastar.ServerSentEventGenerator,
) (error, error) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return nil, nil
}

func (PageIndex) OnEventFuzz(
	event EventFuzz,
	sse *datastar.ServerSentEventGenerator,
) int { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return 0
}
