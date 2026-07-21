//nolint:all
package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
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
	sse datapages.SSE,
	event EventSingular,
) error {
	return nil
}

func (PageIndex) OnPluralUser(
	sse datapages.SSE,
	event EventPluralUser,
	session Session,
) error {
	return nil
}

func (PageIndex) OnSingularUser(
	sse datapages.SSE,
	event EventSingularUser,
	session Session,
) error {
	return nil
}

func (PageIndex) OnMixed(
	sse datapages.SSE,
	event EventMixed,
	session Session,
) error {
	return nil
}

func (PageIndex) OnThreeField(
	sse datapages.SSE,
	event EventThreeField,
	session Session,
) error {
	return nil
}
