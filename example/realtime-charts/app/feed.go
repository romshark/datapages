package app

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/romshark/datapages/example/realtime-charts/market"
	"github.com/romshark/datapages/example/realtime-charts/tsdb"
)

const (
	// period is the length of a bar of the portfolio page in milliseconds.
	period = 5000

	// window is the number of bars of the time charts of the portfolio page: ten minutes.
	window = 120
)

// Signal names with a leading underscore stay out of the requests Datastar sends. Without
// it every settings change would upload the bars, and every stream reconnect would carry
// them in its query.
type signalPatch struct {
	// Indexes updates the URL after a toggle.
	Indexes string `json:"indexes,omitempty"`

	// Feed maps a symbol to the update of its chart. A nil update deletes the signals of
	// a chart the tab no longer shows.
	Feed map[string]*chartPatch `json:"_feed,omitempty"`

	Map       []mapTile       `json:"_map,omitempty"`
	Portfolio *portfolioPatch `json:"_portfolio,omitempty"`

	// Pushed is the Unix second of the push, shown in the navigation bar. Unlike the bars
	// it goes back with every request: a stream that reconnects resumes from it.
	Pushed int64 `json:"pushed,omitempty"`
}

type chartPatch struct {
	// Bars holds only those updated since the previous push, oldest first, as the points
	// the chart type of the tab draws. The <rt-chart> element merges them into what it
	// has drawn.
	Bars   []point `json:"bars"`
	Price  string  `json:"price"`
	Change string  `json:"change"`
	Up     bool    `json:"up"`
}

// point is [time, open, high, low, close] for candles and bars and [time, value] for the
// other chart types, with time in Unix seconds. An array is less than half the size of an
// object with named fields.
type point []float64

type card struct {
	market.Index
	// The change in the card header and a baseline chart refer to Open.
	Open float64
}

func (a *App) cards(symbols []string) []card {
	cards := make([]card, len(symbols))
	for i, sym := range symbols {
		cards[i] = card{market.Indexes[position[sym]], a.market.Open(position[sym])}
	}
	return cards
}

// window returns the updates of the charts of symbols with the bars of the time range rng
// up to the Unix second at. The bar at falls into is the one of that moment, which is
// what a paused page shows.
func (a *App) window(
	t chartType, symbols []string, rng time.Duration, at int64,
) map[string]*chartPatch {
	bar := barOf(rng)
	to, step := at*1000, bar.Milliseconds()
	from := to - to%step - int64(rng/bar-1)*step
	return a.charts(t, symbols, step, from, to)
}

// since returns the updates of the charts of symbols with the bars of the time range rng
// that changed after the Unix second pushed, up to the second now. The bar that was
// forming at pushed is among them.
func (a *App) since(
	t chartType, symbols []string, rng time.Duration, pushed, now int64,
) map[string]*chartPatch {
	from, step := pushed*1000, barOf(rng).Milliseconds()
	return a.charts(t, symbols, step, from-from%step, now*1000)
}

// charts returns the updates of the charts of symbols with the bars of step milliseconds
// between the milliseconds from and to.
func (a *App) charts(
	t chartType, symbols []string, step, from, to int64,
) map[string]*chartPatch {
	feed := make(map[string]*chartPatch, len(symbols))
	for _, sym := range symbols {
		buckets := a.db.OHLC(sym, from, to, step)
		points := make([]point, len(buckets))
		for i, b := range buckets {
			points[i] = pointOf(t, b)
		}
		last, _ := a.db.At(sym, to)
		open := a.market.Open(position[sym])
		feed[sym] = &chartPatch{
			Bars:   points,
			Price:  price(last.Value),
			Change: fmt.Sprintf("%+.2f%%", (last.Value-open)/open*100),
			Up:     last.Value >= open,
		}
	}
	return feed
}

func pointOf(t chartType, b tsdb.Bucket) point {
	sec := float64(b.Time / 1000)
	open, closing := cents(b.Open), cents(b.Close)
	switch t {
	case typeCandles, typeBars:
		return point{sec, open, cents(b.High), cents(b.Low), closing}
	case typeHistogram:
		// The difference of the prices the other charts show. Subtracting two rounded
		// floats can leave a binary fraction such as 0.30000000000000004, which is why
		// the difference is rounded too.
		return point{sec, cents(closing - open)}
	default:
		return point{sec, closing}
	}
}

// cents rounds a price to the precision the charts show.
func cents(v float64) float64 { return math.Round(v*100) / 100 }

func price(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// feedSignal returns the expression that reads field of the chart of in. Optional
// chaining keeps it valid before the first push creates the signal.
func feedSignal(in market.Index, field string) string {
	return "$_feed." + in.Symbol + "?." + field
}

// jsString quotes s for a Datastar expression. Templ escapes the attribute that holds the
// expression.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
