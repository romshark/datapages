package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// The view pages show the same charts with the same settings, each with
// another chart type. [Dashboard] holds their stream.
//
// A page stays as it is when its hidden tab is shown again instead of
// reloading, which Datapages does by default. A reload would end a pause and
// drop the bars the charts have drawn. The stream reconnects and catches up
// on its own, see [Dashboard.StreamOpen].

// PageIndex is /
//
// It draws candlesticks.
type PageIndex struct {
	App *App
	Dashboard
}

func (p PageIndex) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeCandles, query.Values), true, nil
}

// PageBars is /bars
//
// It draws OHLC bars.
type PageBars struct {
	App *App
	Dashboard
}

func (p PageBars) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeBars, query.Values), true, nil
}

// PageLine is /line
//
// It draws a line through the closing prices.
type PageLine struct {
	App *App
	Dashboard
}

func (p PageLine) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeLine, query.Values), true, nil
}

// PageArea is /area
//
// It fills the area below the closing prices.
type PageArea struct {
	App *App
	Dashboard
}

func (p PageArea) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeArea, query.Values), true, nil
}

// PageBaseline is /baseline
//
// It fills the area between the closing prices and the price the market
// opened with, above it in the up color and below it in the down color.
type PageBaseline struct {
	App *App
	Dashboard
}

func (p PageBaseline) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeBaseline, query.Values), true, nil
}

// PageHistogram is /histogram
//
// It draws the change of every bar from its open to its close as a column.
type PageHistogram struct {
	App *App
	Dashboard
}

func (p PageHistogram) GET(
	r *http.Request, query datapages.Query[ViewQuery],
) (body datapages.Component, noRefresh datapages.DisableRefreshAfterHidden, err error) {
	return p.page(typeHistogram, query.Values), true, nil
}
