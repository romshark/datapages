package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// PageOffline is /offline
//
// The service worker stores this page when it installs and serves it while
// offline for every address it has no copy of. At the address of a document,
// the editor opens what the browser stored of it.
// Elsewhere the page lists the documents the browser stores.
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (
	body datapages.Component, head datapages.Head, err error,
) {
	return pageOffline(), headOffline(), nil
}
