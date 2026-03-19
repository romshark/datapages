//nolint:all
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

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// EventSingular is "calc.updated"
type EventSingular struct {
	SubjectInstance string `signal:"instance_id"`

	Result float64 `json:"result"`
}

// EventPluralUser is "chat.sent"
type EventPluralUser struct {
	SubjectUser []string

	Message string `json:"message"`
}

// EventSingularUser is "dm.sent"
type EventSingularUser struct {
	SubjectUser string

	Text string `json:"text"`
}

// EventMixed is "mixed"
type EventMixed struct {
	SubjectUser     []string
	SubjectInstance string `signal:"instance_id"`

	Data string `json:"data"`
}

// EventThreeField is "three"
type EventThreeField struct {
	SubjectUser []string
	SubjectRoom []string
	SubjectCalc string `signal:"calc_id"`

	Payload string `json:"payload"`
}

func (PageIndex) OnSingular(
	sse *datastar.ServerSentEventGenerator,
	event EventSingular,
) error {
	return nil
}

func (PageIndex) OnPluralUser(
	sse *datastar.ServerSentEventGenerator,
	event EventPluralUser,
	session Session,
) error {
	return nil
}

func (PageIndex) OnSingularUser(
	sse *datastar.ServerSentEventGenerator,
	event EventSingularUser,
	session Session,
) error {
	return nil
}

func (PageIndex) OnMixed(
	sse *datastar.ServerSentEventGenerator,
	event EventMixed,
	session Session,
) error {
	return nil
}

func (PageIndex) OnThreeField(
	sse *datastar.ServerSentEventGenerator,
	event EventThreeField,
	session Session,
) error {
	return nil
}
