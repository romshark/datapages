package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// The view pages show the same indexes with the same settings: the chart page as charts
// of the chart type of the settings, the market map among all indexes and the portfolio
// page as a portfolio. [Dashboard] holds their stream. Each page loads the library that
// draws it in its head.
//
// Datapages reloads a page whose hidden tab is shown again. The view pages turn that off:
// a reload would end a pause and drop the bars the charts have drawn. The stream
// reconnects and catches up on its own, see [Dashboard.StreamOpen].

// PageIndex is /
//
// It draws a chart of every index the sidebar selects, as candlesticks, OHLC bars, a line
// through the closing prices, the same line filled below or filled towards the opening
// price, or a column for the change of every bar.
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
// It shows every index as a tile of its sector, colored by its change since the open, and
// outlines the indexes of the sidebar.
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
// It shows the indexes as a portfolio: an equal part of the capital in each, bought at
// its opening price.
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
