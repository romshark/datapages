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
	// period is the portfolio bar length in milliseconds.
	period = 5000

	// window is the 10-minute portfolio window in bars.
	window = 120
)

// pushedSignal is the displayed Unix second.
// Its underscore keeps server-owned state out of Datastar requests.
type pushedSignal struct {
	Pushed int64 `json:"_pushed"`
}

type indexesSignal struct {
	Indexes string `json:"indexes"`
}

type card struct {
	market.Index
	// Open is the reference for the header change and baseline chart.
	Open float64

	// Bars are oldest first. <rt-chart> merges them with existing bars.
	Bars   []point
	Price  string
	Change string
	Up     bool
}

// point is [time, open, high, low, close] or [time, value], with Unix seconds.
// Arrays use less than half the JSON of named objects.
type point []float64

// cards returns chart data through at, including its current bar.
func (a *App) cards(t chartType, symbols []string, rng time.Duration, at int64) []card {
	bar := barOf(rng)
	to, step := at*1000, bar.Milliseconds()
	from := to - to%step - int64(rng/bar-1)*step
	cards := make([]card, len(symbols))
	for i, sym := range symbols {
		buckets := a.db.OHLC(sym, from, to, step)
		bars := make([]point, len(buckets))
		for k, b := range buckets {
			bars[k] = pointOf(t, b)
		}
		last, _ := a.db.At(sym, to)
		open := a.market.Open(position[sym])
		cards[i] = card{
			Index:  market.Indexes[position[sym]],
			Open:   open,
			Bars:   bars,
			Price:  price(last.Value),
			Change: fmt.Sprintf("%+.2f%%", (last.Value-open)/open*100),
			Up:     last.Value >= open,
		}
	}
	return cards
}

func pointOf(t chartType, b tsdb.Bucket) point {
	sec := float64(b.Time / 1000)
	open, closing := cents(b.Open), cents(b.Close)
	switch t {
	case typeCandles, typeBars:
		return point{sec, open, cents(b.High), cents(b.Low), closing}
	case typeHistogram:
		// Round again because subtracting displayed prices can expose a binary fraction.
		return point{sec, cents(closing - open)}
	default:
		return point{sec, closing}
	}
}

func cents(v float64) float64 { return math.Round(v*100) / 100 }

func price(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// jsonOf cannot fail for chart data, which contains only strings and finite numbers.
func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// jsString quotes s before Templ escapes the containing attribute.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
