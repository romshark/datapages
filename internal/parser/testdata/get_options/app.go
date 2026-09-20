package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

// GET without any GET options.
func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// EventPing is "ping"
type EventPing struct {
	N int `json:"n"`
}

// PageStream is /stream
type PageStream struct{ App *App }

func (PageStream) OnPing(event EventPing, sse datapages.SSE) error {
	return nil
}

// GET with enableBackgroundStreaming.
func (PageStream) GET(
	r *http.Request,
) (
	body datapages.Component,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return body, true, nil
}

// PageNoRefresh is /no-refresh
type PageNoRefresh struct{ App *App }

func (PageNoRefresh) OnPing(event EventPing, sse datapages.SSE) error {
	return nil
}

// GET with disableRefreshAfterHidden.
func (PageNoRefresh) GET(
	r *http.Request,
) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	return body, true, nil
}
