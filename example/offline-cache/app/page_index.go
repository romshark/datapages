package app

import (
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
)

// PageIndex is /
//
// The home page is the shows listing with live search.
type PageIndex struct {
	App *App
	Base
}

func (p PageIndex) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	query datapages.Query[SearchParams],
) (body datapages.Component, err error) {
	shows, err := p.App.repo.SearchShows(r.Context(), query.Values.Term)
	if err != nil {
		return nil, err
	}
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}

	// Keep a search-less offline shell for "/" so it renders while offline
	// instead of the generic fallback. Versioned by session so it re-caches with
	// the right navbar after login/logout.
	if ver := offlineCacheVersion(session, ""); pageCache.Version() != ver {
		pageCache.Set(
			href.PageIndex(href.QueryPageIndex{}),
			indexOffline(session, baseData),
			ver,
		)
	}
	return pageShows(session, query.Values, shows, baseData), nil
}

// POSTSearch is /search/{$}
func (p PageIndex) POSTSearch(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[SearchParams],
) error {
	shows, err := p.App.repo.SearchShows(sse.Context(), signals.Values.Term)
	if err != nil {
		return err
	}
	// Patch only the results container so the search input keeps focus.
	return sse.PatchElement(fragmentShowResults(shows))
}
