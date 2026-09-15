package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type TabState struct {
	Filter string
}

// EventFiltersUpdated is "filters.updated"
type EventFiltersUpdated struct {
	Instance datapages.SubjectStateID
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnFiltersUpdated(event EventFiltersUpdated, sse datapages.SSE) error {
	return nil
}

// PageOther is /other
type PageOther struct{ App *App }

func (PageOther) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageOther) StreamOpen(r *http.Request, state datapages.State[TabState]) error {
	return nil
}
