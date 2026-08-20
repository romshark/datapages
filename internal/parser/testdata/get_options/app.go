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

// PageStream is /stream
type PageStream struct{ App *App }

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
