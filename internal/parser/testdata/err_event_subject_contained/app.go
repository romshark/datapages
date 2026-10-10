// Package app declares event fields whose types hold a subject type,
// which are rejected: only a field of the subject type itself routes an event.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// Recipients is declared from a slice of a subject type.
type Recipients []datapages.SubjectUser

// UserID is declared from a subject type. Only a field typed as it is
// rejected for that; a slice of it is rejected like a slice of the type.
type UserID datapages.SubjectUser

// EventSlice is "slice"
type EventSlice struct {
	Recipients []datapages.SubjectUser `json:"recipients"` /* ErrEventSubjectContained */

	Text string `json:"text"`
}

// EventArray is "array"
type EventArray struct {
	Pair [2]datapages.SubjectUser `json:"pair"` /* ErrEventSubjectContained */
}

// EventMapValue is "map.value"
type EventMapValue struct {
	ByIndex map[int]datapages.SubjectUser `json:"by_index"` /* ErrEventSubjectContained */
}

// EventMapKey is "map.key"
type EventMapKey struct {
	ByRoom map[datapages.Subject]string `json:"by_room"` /* ErrEventSubjectContained */
}

// EventPointer is "pointer"
type EventPointer struct {
	Maybe *datapages.SubjectUser `json:"maybe"` /* ErrEventSubjectContained */
}

// EventNamed is "named"
type EventNamed struct {
	To Recipients `json:"to"` /* ErrEventSubjectContained */
}

// EventDerived is "derived"
type EventDerived struct {
	IDs []UserID `json:"ids"` /* ErrEventSubjectContained */
}

// EventStateIDs is "state.ids"
type EventStateIDs struct {
	Tabs []datapages.SubjectStateID `json:"tabs"` /* ErrEventSubjectContained */
}

// EventCascade is "cascade"
//
// The rejected field is no payload field the subject field after it follows.
type EventCascade struct {
	Recipients []datapages.SubjectUser `json:"recipients"` /* ErrEventSubjectContained */
	Room       datapages.Subject       `json:"room"`

	Text string `json:"text"`
}

// EventChan is "chan"
type EventChan struct {
	Feed chan datapages.Subject `json:"feed"` /* ErrEventSubjectContained */
}
