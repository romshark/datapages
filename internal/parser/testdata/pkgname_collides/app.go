//nolint:all

// Package stream is named after a package app_gen.go imports.
// Unaliased, the app import and github.com/romshark/datapages/runtime/stream
// would bind one identifier and the generated package would not compile.
package stream

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventTicked is "ticked"
type EventTicked struct {
	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term string `query:"t"`
	}],
) (body datapages.Component, err error) {
	_ = query
	return body, err
}

// POSTPing is /ping
func (PageIndex) POSTPing(r *http.Request) error { return nil }

func (PageIndex) OnTicked(_ EventTicked, _ datapages.SSE) error { return nil }
