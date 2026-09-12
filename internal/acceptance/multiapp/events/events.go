// Package events holds the event both applications of this module take part in.
//
// Neither app package declares it. Each names the type declared here,
// which puts the two applications on one subject deliberately:
// what admin dispatches reaches the streams of frontend and the other way round.
package events

// EventAnnouncement is "announcement"
type EventAnnouncement struct {
	Text string `json:"text"`
}
