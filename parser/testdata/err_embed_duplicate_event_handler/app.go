package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventSomethingHappened is "something.happened"
type EventSomethingHappened struct{}

type BaseA struct{ App *App }

func (BaseA) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

func (BaseA) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

type BaseB struct{ App *App }

func (BaseB) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	BaseA
	BaseB
}
