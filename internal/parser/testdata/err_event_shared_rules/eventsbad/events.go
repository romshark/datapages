// Package eventsbad declares events that break the event rules.
//
// They are declared outside an app package to check that the rules are applied
// where the type is written, not only where it is used.
package eventsbad

import "github.com/romshark/datapages"

// EventMissingTag is "missing.tag"
type EventMissingTag struct {
	Text string
}

// EventUnexported is "unexported"
type EventUnexported struct {
	text string `json:"text"`
}

// EventSubjectLate is "subject.late"
type EventSubjectLate struct {
	Text string `json:"text"`

	Room datapages.Subject `json:"room"`
}

// EventBadSubject is "bad subject"
type EventBadSubject struct {
	Text string `json:"text"`
}

// EventForUser is "for.user"
type EventForUser struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	Text string `json:"text"`
}

// EventGeneric is "generic"
type EventGeneric[T any] struct {
	Value T `json:"value"`
}
