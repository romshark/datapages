package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// PageOffline is /offline
//
// The service worker precaches this page and serves it for navigations to URLs
// that have no cached copy while the browser is offline.
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	return pageOffline(), true, nil
}
