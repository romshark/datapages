// Package app declares events with more than one datapages.SubjectUser field,
// which are rejected: a stream subscribes by one user position.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request, session Session) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnDMed(event EventDMed, sse datapages.SSE) error { return nil }

func (PageIndex) OnPaid(event EventPaid, sse datapages.SSE) error { return nil }

// EventDMed is "dmed"
//
// Both names of the line are subject fields.
type EventDMed struct {
	To, Cc datapages.SubjectUser /* ErrEventSubjectUserDuplicate */

	Text string `json:"text"`
}

// EventPaid is "paid"
type EventPaid struct {
	From datapages.SubjectUser
	Room datapages.Subject
	To   datapages.SubjectUser /* ErrEventSubjectUserDuplicate */

	Amount int `json:"amount"`
}
