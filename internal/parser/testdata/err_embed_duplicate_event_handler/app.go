package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventSomethingHappened is "something.happened"
type EventSomethingHappened struct{}

type BaseA struct{ App *App }

func (BaseA) OnSomethingHappened(
	event EventSomethingHappened,
	sse datapages.SSE,
) error {
	return nil
}

func (BaseA) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

type BaseB struct{ App *App }

func (BaseB) OnSomethingHappened(
	event EventSomethingHappened,
	sse datapages.SSE,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	BaseA
	BaseB
}
