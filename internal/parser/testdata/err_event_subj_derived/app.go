//nolint:all
package app

import (
	"net/http"

	"datapagestest/fixture/err_event_subj_derived/subpkg"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// UserID is declared from a user segment type.
type UserID datapages.SubjectUser

// Room is declared from a value segment type.
type Room datapages.Subject

// RoomTag is declared from Room, two declarations away from the framework type.
type RoomTag Room

// RoomAlias resolves to the framework type and stays a subject segment.
type RoomAlias = datapages.Subject

// Status is an ordinary string payload type.
type Status string

/* ErrEventSubjectDerivedType: a segment type reached through a declaration
   in the app package, which go/types renders as a plain string */

// EventDerivedUser is "derived_user"
type EventDerivedUser struct {
	To UserID `json:"to"`

	Text string `json:"text"`
}

/* ErrEventSubjectDerivedType: the value segment type, reached through a chain
   of two declarations */

// EventDerivedValue is "derived_value"
type EventDerivedValue struct {
	Room RoomTag `json:"room"`

	Text string `json:"text"`
}

/* ErrEventSubjectDerivedType: declared in an imported package */

// EventDerivedSubpkg is "derived_subpkg"
type EventDerivedSubpkg struct {
	To subpkg.Recipient `json:"to"`

	Text string `json:"text"`
}

// EventPlain is "plain"
//
// An alias is the framework type itself and a plain string type is a payload
// field: neither is reported.
type EventPlain struct {
	Room RoomAlias `json:"room"`

	Status Status `json:"status"`
}
