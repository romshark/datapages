package app

import (
	"net/http"

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

func (PageIndex) GET(r *http.Request) error { return nil }
