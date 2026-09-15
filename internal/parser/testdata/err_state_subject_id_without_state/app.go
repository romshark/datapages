package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

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
