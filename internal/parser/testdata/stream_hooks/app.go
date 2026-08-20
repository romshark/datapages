package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// EventPing is "ping"
type EventPing struct{}

type Base struct{ App *App }

func (Base) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE,
	session Session,
	signals datapages.Signals[struct {
		Instance string `json:"instance"`
	}],
	dispatch datapages.Dispatcher[EventPing],
) error {
	return nil
}

func (Base) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	session Session,
	dispatch datapages.Dispatcher[EventPing],
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageStreamMin is /stream-min
type PageStreamMin struct{ App *App }

func (PageStreamMin) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageStreamMin) StreamOpen(
	r *http.Request,
	id datapages.StreamID,
) {
}

func (PageStreamMin) StreamClose(
	r *http.Request,
	id datapages.StreamID,
) {
}

// PageStreamMax is /stream-max
type PageStreamMax struct{ App *App }

func (PageStreamMax) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageStreamMax) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE,
	session Session,
	signals datapages.Signals[struct {
		Instance string `json:"instance"`
	}],
	dispatch datapages.Dispatcher[EventPing],
) error {
	return nil
}

func (PageStreamMax) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	session Session,
	dispatch datapages.Dispatcher[EventPing],
) error {
	return nil
}

func (PageStreamMax) OnPing(
	event EventPing,
	sse datapages.SSE,
	streamID datapages.StreamID,
) error {
	return nil
}
