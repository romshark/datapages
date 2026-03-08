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
type EventPing struct {
	Data string `json:"data"`
}

// PageIndex is /
type PageIndex struct{ App *App }

// GET with conventional order.
func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageSessionFirst is /session-first
type PageSessionFirst struct{ App *App }

// GET with session before sessionToken (reversed from conventional).
func (PageSessionFirst) GET(
	session Session,
	sessionToken string,
	r *http.Request,
) (body templ.Component, err error) {
	_ = session
	_ = sessionToken
	return body, err
}

// PageReversed is /reversed/{id}
type PageReversed struct{ App *App }

// GET with all params in reverse order.
func (PageReversed) GET(
	query struct {
		Page int `query:"page"`
	},
	path struct {
		ID string `path:"id"`
	},
	session Session,
	sessionToken string,
	r *http.Request,
) (body templ.Component, err error) {
	_ = query
	_ = path
	_ = session
	_ = sessionToken
	return body, err
}

// PageSignalsFirst is /signals-first
type PageSignalsFirst struct{ App *App }

// GET with signals before request.
func (PageSignalsFirst) GET(
	signals struct {
		Search string `json:"search"`
	},
	r *http.Request,
) (body templ.Component, err error) {
	_ = signals
	return body, err
}

// PageActionReversed is /action-reversed
type PageActionReversed struct{ App *App }

func (PageActionReversed) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// POSTSubmit is /action-reversed/submit
//
// Action with SSE and session in reversed order.
func (PageActionReversed) POSTSubmit(
	session Session,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		Name string `json:"name"`
	},
	r *http.Request,
) error {
	_ = session
	_ = sse
	_ = signals
	return nil
}

// PageEventReversed is /event-reversed
type PageEventReversed struct{ App *App }

func (PageEventReversed) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// Event handler with SSE before event (reversed from conventional).
func (PageEventReversed) OnEventPing(
	sse *datastar.ServerSentEventGenerator,
	session Session,
	event EventPing,
) error {
	_ = sse
	_ = session
	_ = event
	return nil
}
