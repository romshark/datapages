// Package app covers stream hooks that take state but no streamID.
//
// A stream hook needs a handle on the stream it is about.
// datapages.StreamID names the stream and datapages.State[T] is the value
// that belongs to it. Either one on its own is enough.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// TabState is the per-tab state of PageIndex.
type TabState struct{ Count int }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// StreamOpen takes state and no streamID.
func (PageIndex) StreamOpen(
	r *http.Request,
	state datapages.State[TabState],
) error {
	_ = r
	state.Values.Count = 1
	return nil
}

// StreamClose takes state and no streamID.
func (PageIndex) StreamClose(
	r *http.Request,
	state datapages.State[TabState],
) error {
	_, _ = r, state
	return nil
}

// PageBoth is /both
//
// Both handles at once stay accepted.
type PageBoth struct{ App *App }

func (PageBoth) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageBoth) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[TabState],
) error {
	_, _, _ = r, streamID, state
	return nil
}
