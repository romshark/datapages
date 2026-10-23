// Package app takes struct types from other modules into an event and into
// signals: url.URL and sql.NullString. encoding/json decides their JSON, which
// leaves their fields out of the json-tag rules.
package app

import (
	"database/sql"
	"net/http"
	"net/url"

	"github.com/romshark/datapages"
)

type App struct{}

// EventLinked is "linked"
type EventLinked struct {
	Link url.URL  `json:"link"`
	Prev *url.URL `json:"prev"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIndex) OnLinked(event EventLinked, sse datapages.SSE) error { return nil }

// POSTSave is /save
func (PageIndex) POSTSave(
	r *http.Request,
	signals datapages.Signals[struct {
		Name sql.NullString `json:"name"`
	}],
) error {
	_ = signals
	return nil
}
