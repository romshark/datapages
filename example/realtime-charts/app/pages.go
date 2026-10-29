package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// The view pages share [Dashboard] and disable visibility reloads.
// Their streams catch up without ending a pause or dropping bars already drawn.

// PageIndex is /
//
// It shows selected indexes in 6 chart types.
type PageIndex struct {
	App *App
	Dashboard
}

func (p PageIndex) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (
	body datapages.Component, head datapages.Head,
	noRefresh datapages.DisableRefreshAfterHidden, err error,
) {
	return p.page(viewCharts, query.Values), chartsHead(), true, nil
}

// PageMap is /map
//
// It shows every index by sector and outlines selected indexes.
type PageMap struct {
	App *App
	Dashboard
}

func (p PageMap) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (
	body datapages.Component, head datapages.Head,
	noRefresh datapages.DisableRefreshAfterHidden, err error,
) {
	return p.page(viewMap, query.Values), mapHead(), true, nil
}

// PagePortfolio is /portfolio
//
// It invests an equal part of the capital in each selected index.
type PagePortfolio struct {
	App *App
	Dashboard
}

func (p PagePortfolio) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (
	body datapages.Component, head datapages.Head,
	noRefresh datapages.DisableRefreshAfterHidden, err error,
) {
	return p.page(viewPortfolio, query.Values), portfolioHead(), true, nil
}
