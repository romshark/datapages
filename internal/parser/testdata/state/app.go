package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PUTAppLevel is /app-level
//
// App-level action that takes per-tab state. The runtime resolves the
// instance-id header to a slot in the StateIndex map; calls from tabs
// not bound to StateIndex are rejected with 409.
func (*App) PUTAppLevel(
	r *http.Request,
	state *StateIndex,
) error {
	_ = state
	return nil
}

// EventPing is "ping"
type EventPing struct{}

// StateIndex is the per-instance state for PageIndex.
type StateIndex struct {
	Counter int
}

// TabContext is the per-instance state for Base.
//
// Note: the type name deliberately does not start with "State" — the
// parser identifies state types by how they are used (parameter name
// and pointer-to-struct shape), not by naming convention.
type TabContext struct {
	Started bool
}

type Base struct{ App *App }

func (Base) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *TabContext,
) error {
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageIndex) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse datapages.SSE,
	state *StateIndex,
) error {
	return nil
}

func (PageIndex) StreamClose(
	r *http.Request,
	streamID uint64,
	state *StateIndex,
) error {
	return nil
}

// POSTIncrement is /inc
func (PageIndex) POSTIncrement(
	r *http.Request,
	state *StateIndex,
	dispatch func(EventPing) error,
) error {
	return nil
}

func (PageIndex) OnPing(
	event EventPing,
	sse datapages.SSE,
	state *StateIndex,
) error {
	return nil
}

// PageBase is /base
type PageBase struct {
	App *App
	Base
}

func (PageBase) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
