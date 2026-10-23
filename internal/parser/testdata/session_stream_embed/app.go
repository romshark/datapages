// Package app declares the session-carrying handlers a stream embed takes.
// The concrete page only embeds them.
//
// The parser finds the Session type anyway, accepts EventDM,
// and the generated package compiles.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type SessionData struct{ Name string }

// EventDM is "dm"
//
// A user-addressed event needs a Session type, which only a stream embed declares.
type EventDM struct {
	To datapages.SubjectUser

	Text string `json:"text"`
}

// Base carries the session-taking stream hooks and event handler.
type Base struct{ App *App }

func (Base) StreamOpen(
	_ *http.Request,
	streamID datapages.StreamID,
	s datapages.Session[SessionData],
) error {
	return nil
}

func (Base) StreamClose(
	_ *http.Request,
	streamID datapages.StreamID,
	s datapages.Session[SessionData],
) error {
	return nil
}

func (Base) OnDM(
	event EventDM,
	sse datapages.SSE,
	s datapages.Session[SessionData],
) error {
	return nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
