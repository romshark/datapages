package app

import (
	"net/http"

	"github.com/a-h/templ"
)

// PageOffline is /offline
//
// The service worker precaches this page and serves it for navigations to URLs
// that have no cached copy while the browser is offline.
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (
	body templ.Component,
	disableRefreshAfterHidden bool,
	err error,
) {
	return pageOffline(), true, nil
}
