package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

type Session struct {
	UserID   string
	IssuedAt time.Time
}

// EventPing is "ping"
type EventPing struct{}

type Base struct{ App *App }

func (Base) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse *datastar.ServerSentEventGenerator,
	sessionToken string,
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
	sessionToken string,
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

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// PageStreamMin is /stream-min
type PageStreamMin struct{ App *App }

func (PageStreamMin) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

func (PageStreamMin) StreamOpen(
	r *http.Request,
	streamID uint64,
) error {
	return nil
}

func (PageStreamMin) StreamClose(
	r *http.Request,
	streamID uint64,
) error {
	return nil
}

// PageStreamMax is /stream-max
type PageStreamMax struct{ App *App }

func (PageStreamMax) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

func (PageStreamMax) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse *datastar.ServerSentEventGenerator,
	sessionToken string,
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
	sessionToken string,
	session Session,
	dispatch func(EventPing) error,
) error {
	return nil
}
