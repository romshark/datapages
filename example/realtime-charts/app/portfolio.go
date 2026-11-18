package app

import (
	"fmt"
	"math"
	"strconv"

	"github.com/romshark/datapages/example/realtime-charts/market"
)

const capital = 100_000

// portfolioData contains the page panels. Time-series rows are [Unix second, values...].
type portfolioData struct {
	// Returns compares the portfolio with all indexes.
	// Allocation holds each position's percentage of portfolio value.
	Returns    []point
	Allocation []point

	// Drawdown is how far the portfolio is below its high within the window, in percent.
	Drawdown []point

	// Distribution rows are [bin center in percent, count].
	Distribution []point

	// Profit holds each position's gain or loss in dollars.
	Profit []bar

	Value                       string
	Change, Market, Best, Worst tile
}

// bar is [label, value].
type bar [2]any

type tile struct {
	Text string
	Up   bool
	Note string
}

// holding is the shares bought by 1 equal part of capital at the opening price.
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

// portfolio returns panels through at. A portfolio without holdings shows nothing.
func (a *App) portfolio(symbols []string, at int64) portfolioData {
	var p portfolioData
	if len(symbols) == 0 {
		return p
	}
	hs := a.holdings(symbols)
	to := at * 1000
	from := to - to%period - (window-1)*period
	times, closes := a.closes(symbols, from, to)
	if len(times) == 0 {
		return p
	}
	values := make([]float64, len(times))
	for k := range times {
		for i, h := range hs {
			values[k] += h.shares * closes[i][k]
		}
	}

	average := a.marketReturns(from, to)
	for k, t := range times {
		m, ok := average[t]
		if !ok {
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

// closes returns bars common to every symbol. A stream can read between per-index writes.
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

// marketReturns maps bar times to the average index return since open, in percent.
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

// distribution bins consecutive percentage returns around zero as [center, count].
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

// binWidth splits 0..extreme into at most 8 round bins.
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

// share is a sector's part of opening capital.
type share struct {
	Sector string
	// Color indexes [market.Sectors].
	Color int
	// Offset starts the ring segment. Dash excludes its trailing gap.
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

func chartID(kind string, holdings []string) string {
	return "pf-" + kind + "-" + formatIndexes(holdings)
}

func colorClass(i int) string { return "c" + strconv.Itoa(i%10) }

func percent(v, base float64) float64 { return (v/base - 1) * 100 }

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

// dollars rounds to whole dollars and adds thousands separators.
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
