// Package app declares every session-carrying handler shape on abstract pages.
// The concrete pages only embed them.
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
// A user-addressed event needs a Session type, which only an abstract page declares.
type EventDM struct {
	To datapages.SubjectUser

	Text string `json:"text"`
}

// Base carries the session-taking GET, stream hooks and event handler.
type Base struct{ App *App }

func (Base) GET(
	_ *http.Request,
	s datapages.Session[SessionData],
) (body datapages.Component, err error) {
	_ = s.Data().Name
	return nil, nil
}

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

// SignIn carries the actions that open and close a session.
type SignIn struct{ App *App }

// POSTSignIn is /sign-in
func (SignIn) POSTSignIn(_ *http.Request) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	return newSession, datapages.Redirect{URL: "/"}, nil
}

// POSTSignOut is /sign-out
func (SignIn) POSTSignOut(_ *http.Request) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/"}, nil
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

// PageAuth is /auth
type PageAuth struct {
	App *App
	SignIn
}

func (PageAuth) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
