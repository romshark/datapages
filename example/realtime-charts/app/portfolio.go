package app

import (
	"fmt"
	"math"
	"strconv"

	"github.com/romshark/datapages/example/realtime-charts/market"
)

// capital is what the portfolio invests at the open, in dollars.
const capital = 100_000

// A time series of a portfolioPatch is rows [time, value...] with time in Unix seconds,
// one row per bar, which the <pf-chart> element merges by time. The other fields replace
// what their panel shows.
type portfolioPatch struct {
	// Returns compares the portfolio with all indexes together since the open, in
	// percent. Allocation is the share of every holding in the value of the portfolio, in
	// percent. Both carry the bars that changed since the previous push.
	Returns    []point `json:"returns"`
	Allocation []point `json:"allocation"`

	// Drawdown is how far the portfolio is below its high within the window, in percent.
	// It carries every bar of the window: a new high moves the rows after it.
	Drawdown []point `json:"drawdown"`

	// Distribution counts the returns of the portfolio from one bar to the next within
	// the window, by bins of equal width: rows [center in percent, count].
	Distribution []point `json:"distribution"`

	// Profit holds the gain or loss of every holding, in dollars.
	Profit []bar `json:"profit"`

	Value  string `json:"value"`
	Change tile   `json:"change"`
	Market tile   `json:"market"`
	Best   tile   `json:"best"`
	Worst  tile   `json:"worst"`
}

// bar is [label, value], the column of a category.
type bar [2]any

type tile struct {
	Text string `json:"text"`
	Up   bool   `json:"up"` // shows a gain
	Note string `json:"note,omitempty"`
}

// holding is an index of the portfolio: the shares that an equal part of the capital
// bought at its opening price.
type holding struct {
	symbol string
	open   float64
	shares float64
}

func (a *App) holdings(symbols []string) []holding {
	hs := make([]holding, len(symbols))
	for i, sym := range symbols {
		open := a.market.Open(position[sym])
		hs[i] = holding{sym, open, capital / float64(len(symbols)) / open}
	}
	return hs
}

// portfolio returns the update of the panels of the portfolio of symbols with the window
// of bars up to the Unix second at. The returns and the allocation carry the bars that
// changed after the Unix second since, all of them when since is zero. A portfolio
// without holdings has no update.
func (a *App) portfolio(symbols []string, since, at int64) *portfolioPatch {
	if len(symbols) == 0 {
		return nil
	}
	hs := a.holdings(symbols)
	to := at * 1000
	from := to - to%period - (window-1)*period
	times, closes := a.closes(symbols, from, to)
	if len(times) == 0 {
		return nil
	}
	values := make([]float64, len(times))
	for k := range times {
		for i, h := range hs {
			values[k] += h.shares * closes[i][k]
		}
	}

	// The bars since the previous push, among them the one forming then.
	first := from
	if since != 0 {
		first = max(from, since*1000-since*1000%period)
	}
	average := a.marketReturns(first, to)
	p := &portfolioPatch{}
	for k, t := range times {
		m, ok := average[t]
		if t < first || !ok {
			continue
		}
		sec := float64(t / 1000)
		p.Returns = append(p.Returns,
			point{sec, round(percent(values[k], capital), 2), round(m, 2)})
		row := point{sec}
		for i, h := range hs {
			row = append(row, round(100*h.shares*closes[i][k]/values[k], 2))
		}
		p.Allocation = append(p.Allocation, row)
	}

	peak := values[0]
	for k, v := range values {
		peak = max(peak, v)
		p.Drawdown = append(p.Drawdown,
			point{float64(times[k] / 1000), round(percent(v, peak), 2)})
	}
	p.Distribution = distribution(values)

	// The tiles and the profit show the prices at the last bar, which the charts end
	// with.
	last := len(times) - 1
	value := values[last]
	p.Value = dollars(value)
	p.Change = tile{
		Text: fmt.Sprintf("%+.2f%%", percent(value, capital)),
		Up:   value >= capital,
	}
	if m, ok := average[times[last]]; ok {
		p.Market = tile{
			Text: fmt.Sprintf("%+.2f%%", m),
			Up:   m >= 0,
			Note: fmt.Sprintf("Equal parts of all %d indexes", len(everyIndex)),
		}
	}
	best, worst := 0, 0
	for i, h := range hs {
		price := closes[i][last]
		p.Profit = append(p.Profit, bar{h.symbol, cents(h.shares * (price - h.open))})
		if price/h.open > closes[best][last]/hs[best].open {
			best = i
		}
		if price/h.open < closes[worst][last]/hs[worst].open {
			worst = i
		}
	}
	p.Best = holdingTile(hs[best], closes[best][last])
	p.Worst = holdingTile(hs[worst], closes[worst][last])
	return p
}

// closes returns the start times of the bars of from..to that every index of symbols has,
// and the closing prices of every index in those bars. Every index has a price at the
// same times, but a stream can read the database while the market writes one index after
// the other.
func (a *App) closes(symbols []string, from, to int64) ([]int64, [][]float64) {
	byTime := make([]map[int64]float64, len(symbols))
	for i, sym := range symbols {
		buckets := a.db.OHLC(sym, from, to, period)
		byTime[i] = make(map[int64]float64, len(buckets))
		for _, b := range buckets {
			byTime[i][b.Time] = b.Close
		}
	}
	var times []int64
	closes := make([][]float64, len(symbols))
	for _, b := range a.db.OHLC(symbols[0], from, to, period) {
		all := true
		for i := range symbols {
			if _, ok := byTime[i][b.Time]; !ok {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		times = append(times, b.Time)
		for i := range symbols {
			closes[i] = append(closes[i], byTime[i][b.Time])
		}
	}
	return times, closes
}

// marketReturns maps the start time of every bar of from..to to the average return of all
// indexes since the open, in percent, which the portfolio page compares the portfolio
// with.
func (a *App) marketReturns(from, to int64) map[int64]float64 {
	times, closes := a.closes(everyIndex, from, to)
	returns := make(map[int64]float64, len(times))
	for k, t := range times {
		var sum float64
		for i := range everyIndex {
			sum += closes[i][k] / a.market.Open(i)
		}
		returns[t] = percent(sum/float64(len(everyIndex)), 1)
	}
	return returns
}

func holdingTile(h holding, price float64) tile {
	return tile{
		Text: fmt.Sprintf("%s %+.2f%%", h.symbol, percent(price, h.open)),
		Up:   price >= h.open,
		Note: market.Indexes[position[h.symbol]].Name,
	}
}

// distribution counts the returns of values from one to the next, in percent, by bins of
// equal width centered on zero. It returns a row [center, count] for every bin from the
// lowest return to the highest.
func distribution(values []float64) []point {
	if len(values) < 2 {
		return nil
	}
	returns := make([]float64, len(values)-1)
	var extreme float64
	for k := 1; k < len(values); k++ {
		returns[k-1] = percent(values[k], values[k-1])
		extreme = max(extreme, math.Abs(returns[k-1]))
	}
	width := binWidth(extreme)
	side := int(math.Ceil(extreme / width))
	counts := make([]float64, 2*side+1)
	for _, r := range returns {
		counts[side+int(math.Round(r/width))]++
	}
	rows := make([]point, len(counts))
	for i, c := range counts {
		rows[i] = point{round(float64(i-side)*width, 4), c}
	}
	return rows
}

// binWidth returns a round width, 1, 2 or 5 times a power of ten, that splits 0..extreme
// into at most eight bins.
func binWidth(extreme float64) float64 {
	if extreme <= 0 {
		return 0.01
	}
	raw := extreme / 8
	pow := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, f := range []float64{1, 2, 5} {
		if raw <= f*pow {
			return f * pow
		}
	}
	return 10 * pow
}

// share is the part of the capital the holdings of a sector got at the open.
type share struct {
	Sector string
	// Color is the position of the sector in [market.Sectors].
	Color int
	// Offset is the sum of the shares before this one. Dash is the part of the ring the
	// share covers, less a gap before the next one.
	Percent, Offset, Dash float64
}

func sectorShares(symbols []string) []share {
	counts := map[string]int{}
	for _, sym := range symbols {
		counts[market.Indexes[position[sym]].Sector]++
	}
	var shares []share
	var offset float64
	for i, sector := range market.Sectors {
		if n := counts[sector]; n > 0 {
			p := 100 * float64(n) / float64(len(symbols))
			shares = append(shares, share{sector, i, p, offset, p})
			offset += p
		}
	}
	if len(shares) > 1 {
		for i := range shares {
			shares[i].Dash -= 0.8
		}
	}
	return shares
}

// A time chart of the portfolio page keeps windowSpan seconds of rows.
var windowSpan = strconv.Itoa(window * period / 1000)

func chartID(kind string, holdings []string) string {
	return "pf-" + kind + "-" + formatIndexes(holdings)
}

func colorClass(i int) string { return "c" + strconv.Itoa(i%10) }

func percent(v, base float64) float64 { return (v/base - 1) * 100 }

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

// dollars formats v as whole dollars with a comma between thousands.
func dollars(v float64) string {
	s := strconv.FormatInt(int64(math.Round(math.Abs(v))), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if v <= -0.5 {
		return "-$" + s
	}
	return "$" + s
}
