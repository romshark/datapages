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
	streamID uint64,
	sse datapages.SSE,
	session Session,
	signals struct {
		Instance string `json:"instance"`
	},
	dispatch func(EventPing) error,
) error {
	return nil
}

func (Base) StreamClose(
	r *http.Request,
	streamID uint64,
	session Session,
	dispatch func(EventPing) error,
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
	streamID uint64,
) {
}

func (PageStreamMin) StreamClose(
	r *http.Request,
	streamID uint64,
) {
}

// PageStreamMax is /stream-max
type PageStreamMax struct{ App *App }

func (PageStreamMax) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageStreamMax) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse datapages.SSE,
	session Session,
	signals struct {
		Instance string `json:"instance"`
	},
	dispatch func(EventPing) error,
) error {
	return nil
}

func (PageStreamMax) StreamClose(
	r *http.Request,
	streamID uint64,
	session Session,
	dispatch func(EventPing) error,
) error {
	return nil
}

func (PageStreamMax) OnPing(
	event EventPing,
	sse datapages.SSE,
	streamID uint64,
) error {
	return nil
}
