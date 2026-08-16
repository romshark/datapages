package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// EventPing is "ping"
type EventPing struct {
	Data string `json:"data"`
}

func (*App) Head(r *http.Request, session Session) datapages.Component {
	_ = session
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

// GET without session.
func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// PageProfile is /profile
type PageProfile struct{ App *App }

// GET with session.
func (PageProfile) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, err error) {
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
	sse datapages.SSE,
	session Session,
) error {
	_ = sse
	_ = session
	return nil
}

// Event handler with session.
func (PageProfile) OnEventPing(
	event EventPing,
	sse datapages.SSE,
	session Session,
) error {
	_ = event
	_ = sse
	_ = session
	return nil
}

// PageSettings is /settings
type PageSettings struct{ App *App }

// GET with session.
func (PageSettings) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

// POSTClose is /settings/close
//
// Action with session.
func (PageSettings) POSTClose(
	r *http.Request,
	session Session,
) error {
	_ = session
	return nil
}

// Event handler with session.
func (PageSettings) OnEventPing(
	event EventPing,
	sse datapages.SSE,
	session Session,
) error {
	_ = event
	_ = sse
	_ = session
	return nil
}
