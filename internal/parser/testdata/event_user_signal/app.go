// Package app has an event with a user-addressed subject field and a
// signal-tagged subject field, the shape SPECIFICATION.md shows for
// EventRoomUpdate.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type Session = datapages.Session[struct{}]

type App struct{}

// EventRoomUpdate is "room.update"
type EventRoomUpdate struct {
	Recipient datapages.SubjectUser `json:"recipient"`
	Calc      datapages.Subject     `json:"calc" signal:"calc_id"`

	Data string `json:"data"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	return nil, nil
}

func (PageIndex) OnRoomUpdate(event EventRoomUpdate, sse datapages.SSE) error {
	return nil
}
