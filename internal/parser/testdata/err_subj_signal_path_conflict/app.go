package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventChatSaid is "chat.said"
type EventChatSaid struct {
	Chat datapages.Subject `signal:"chat"`
}

// EventRoomSaid is "room.said"
type EventRoomSaid struct {
	Room datapages.Subject `signal:"chat.room"`
}

// PageIndex is /
//
// The page subscribes by the signal chat and by chat.room. Datastar holds chat
// either as a value or as the object holding room, and the parser rejects this.
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnChatSaid(event EventChatSaid, sse datapages.SSE) error {
	return nil
}

func (PageIndex) OnRoomSaid(event EventRoomSaid, sse datapages.SSE) error {
	return nil
}
