// Package app declares two events with the same subject.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventA is "same"
type EventA struct {
	X string `json:"x"`
}

// EventB is "same"
type EventB struct {
	Y string `json:"y"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) OnA(event EventA, sse datapages.SSE) error { return nil }

func (PageIndex) OnB(event EventB, sse datapages.SSE) error { return nil }
