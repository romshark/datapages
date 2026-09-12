// Package events holds events that are not declared in an app package.
//
// Two applications of one module take part in one event by
// naming the type declared here. The subject, the payload and the
// subject fields are read where the type is written.
package events

import "github.com/romshark/datapages"

// EventShared is "shared"
type EventShared struct {
	Text string `json:"text"`
}

// EventRoom is "room"
type EventRoom struct {
	Room datapages.Subject `json:"room"`

	Text string `json:"text"`
}

// EventCalc is "calc"
//
// A signal-scoped subject field, which the stream fills from a signal the
// client sends rather than from the route or the session.
type EventCalc struct {
	Calc datapages.Subject `json:"calc" signal:"calc_id"`

	Text string `json:"text"`
}
