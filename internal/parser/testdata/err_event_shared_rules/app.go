package app

import (
	"net/http"

	"datapagestest/fixture/err_event_shared_rules/eventsbad"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIndex) OnMissingTag(event eventsbad.EventMissingTag, sse datapages.SSE) error {
	return nil
}

func (PageIndex) OnUnexported(event eventsbad.EventUnexported, sse datapages.SSE) error {
	return nil
}

func (PageIndex) OnSubjectLate(event eventsbad.EventSubjectLate, sse datapages.SSE) error {
	return nil
}

func (PageIndex) OnBadSubject(event eventsbad.EventBadSubject, sse datapages.SSE) error {
	return nil
}

// OnGeneric handles a generic event type, which generated code cannot name.
func (PageIndex) OnGeneric(
	event eventsbad.EventGeneric[string], sse datapages.SSE,
) error {
	return nil
}

// OnForUser handles a user-addressed event in an application with no session.
func (PageIndex) OnForUser(event eventsbad.EventForUser, sse datapages.SSE) error {
	return nil
}
