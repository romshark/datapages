package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

type Session struct {
	UserID string
}

// EventPing is "ping"
type EventPing struct {
	Data string `json:"data"`
}

// PageIndex is /
type PageIndex struct{ App *App }

// GET without session.
func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// PageProfile is /profile
type PageProfile struct{ App *App }

// GET with session.
func (PageProfile) GET(
	r *http.Request,
	session Session,
) (body templ.Component, err error) {
	_ = session
	return body, err
}

// POSTUpdate is /profile/update
//
// Action with session.
func (PageProfile) POSTUpdate(
	r *http.Request,
	session Session,
) error {
	_ = session
	return nil
}

// POSTNotify is /profile/notify
//
// Action with SSE and session.
func (PageProfile) POSTNotify(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	_ = sse
	_ = session
	return nil
}

// Event handler with session.
func (PageProfile) OnEventPing(
	event EventPing,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	_ = event
	_ = sse
	_ = session
	return nil
}
