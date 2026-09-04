//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// EventSingular is "calc.updated"
type EventSingular struct {
	Instance datapages.Subject `signal:"instance_id"`

	Result float64 `json:"result"`
}

// EventPluralUser is "chat.sent"
type EventPluralUser struct {
	Recipient datapages.SubjectUser

	Message string `json:"message"`
}

// EventSingularUser is "dm.sent"
type EventSingularUser struct {
	Recipient datapages.SubjectUser

	Text string `json:"text"`
}

// EventMixed is "mixed"
type EventMixed struct {
	Recipient datapages.SubjectUser
	Instance  datapages.Subject `signal:"instance_id"`

	Data string `json:"data"`
}

// EventThreeField is "three"
type EventThreeField struct {
	Recipient datapages.SubjectUser
	Room      datapages.Subject
	Calc      datapages.Subject `signal:"calc_id"`

	Payload string `json:"payload"`
}

// EventMultiUser is "multi"
type EventMultiUser struct {
	To, Cc datapages.SubjectUser

	Text string `json:"text"`
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

func (PageIndex) OnMultiUser(
	sse datapages.SSE,
	event EventMultiUser,
	session Session,
) error {
	return nil
}
