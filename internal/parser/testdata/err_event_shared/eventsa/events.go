// Package eventsa declares an event another package declares by the same name.
package eventsa

// EventDup is "a.dup"
type EventDup struct {
	Text string `json:"text"`
}
