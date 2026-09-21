package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	pageCache.Set("/", nil, 1)
	return nil, nil
}

// POSTReset is /reset
func (PageIndex) POSTReset(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (redirect datapages.Redirect, err error) {
	pageCache.ClearAll()
	return datapages.Redirect{URL: "/"}, nil
}

// PageError404 is /not-found
//
// A page cache write from the page rendered inline for an unclaimed URL.
type PageError404 struct{ App *App }

func (PageError404) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	pageCache.Clear(r.URL.Path)
	return nil, nil
}

// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
