// Package eventsb declares an event another package declares
// by the same name, and one without a subject comment.
package eventsb

// EventDup is "b.dup"
type EventDup struct {
	Text string `json:"text"`
}

type EventNoComment struct {
	Text string `json:"text"`
}
