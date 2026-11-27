// Tests compute expected portfolios independently from the database.

package app_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/realtime-charts/market"
)

// portfolio is the rendered panel data.
type portfolio struct {
	Returns      [][]float64
	Allocation   [][]float64
	Drawdown     [][]float64
	Distribution [][]float64
	Profit       [][2]any
	Value        string
	Change       tile
	Market       tile
	Best         tile
	Worst        tile
}

type tile struct {
	Text string
	Up   bool
	Note string
}

// TestPortfolioPage tests the complete initial portfolio.
func TestPortfolioPage(t *testing.T) {
	e := newEnv(t)
	resp := e.get(t, views["portfolio"].href(app.ViewQuery{}))
	require.Equal(t, http.StatusOK, resp.Status)
	doc := resp.Document()

	require.Equal(t, "portfolio", resp.Signals(t)["view"])
	for _, kind := range []string{
		"spark", "lines", "stacked", "bars", "area", "histogram",
	} {
		one(t, doc, "the "+kind+" chart", tag("pf-chart"), attr("kind", kind))
	}
	// The IDs of the charts name the holdings.
	resp.ElementByID(t, "pf-returns-AURA-BLTX-CRUX-DUNS-ECHN-FERN")
	require.Equal(t, "AURA,BLTX,CRUX,DUNS,ECHN,FERN",
		attrOf(one(t, doc, "the allocation chart", attr("kind", "stacked")), "labels"))
	e.requirePortfolio(t, portfolioOf(t, doc), defaults, t0.Unix())
	require.Equal(t, map[string]string{
		"Technology": "17%", "Energy": "17%", "Health": "17%",
		"Materials": "17%", "Communication": "17%", "Consumer": "17%",
	}, sharesOf(t, doc))
	requireSidebar(t, resp.ElementByID(t, "sidebar"), defaults)
	requireTabs(t, resp.ElementByID(t, "tabs"), "portfolio",
		app.ViewQuery{Type: "candles", Layout: "grid", Refresh: "1s", Range: "15m", Indexes: indexesParam(defaults)})

	// Every page loads only the library that draws it, and credits it.
	one(t, doc,
		"the uPlot script", tag("script"),
		attr("src", "https://cdn.jsdelivr.net/npm/uplot@1.6.32/dist/uPlot.iife.min.js"))
	one(t, doc,
		"the chart module", tag("script"), attr("src", assets.Path("pf-chart.js")))
	one(t, doc,
		"the uPlot credit", tag("a"), attr("href", "https://github.com/leeoniya/uPlot"))
	require.NotContains(t, resp.Body, "lightweight-charts")
	charts := e.get(t, views["charts"].href(app.ViewQuery{}))
	one(t, charts.Document(), "the Lightweight Charts script", tag("script"),
		attrContains("src", "/lightweight-charts@"))
	one(t, charts.Document(), "the Lightweight Charts credit", tag("a"),
		attr("href", "https://github.com/tradingview/lightweight-charts"))
	// The license of Lightweight Charts asks for a link to tradingview.com on the page.
	// The charts turn off the logo that would carry it.
	one(t, charts.Document(), "the TradingView link", tag("a"),
		attr("href", "https://www.tradingview.com/"))
	require.NotContains(t, charts.Body, "uPlot")

	empty := e.get(t, views["portfolio"].href(app.ViewQuery{Indexes: "none"})).Document()
	one(t, empty, "the empty view", class("empty"))
	require.Empty(t, find(empty, tag("pf-chart")))
}

// TestPortfolioStream tests full-window pushes.
func TestPortfolioStream(t *testing.T) {
	e := newEnv(t)
	holdings := []string{"AURA", "GLWT", "VRGE"}
	tb := e.openTab(t, views["portfolio"].href(app.ViewQuery{}),
		`{"view":"portfolio","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-GLWT-VRGE"}`)
	r := tb.renders(t, 1)[0]
	require.Equal(t, t0.Unix(), r.At)
	e.requirePortfolio(t, portfolioOf(t, r.Main), holdings, t0.Unix())

	for push := 1; push <= 3; push++ {
		e.tick(t, push)
		r := tb.renders(t, push+1)[push]
		now := t0.Unix() + int64(push)
		require.Equal(t, now, r.At)
		e.requirePortfolio(t, portfolioOf(t, r.Main), holdings, now)
	}
}

// TestPortfolioToggle tests replacing panels after a holding changes.
func TestPortfolioToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["portfolio"].href(app.ViewQuery{}),
		`{"view":"portfolio","type":"candles","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.renders(t, 1)

	evs := tb.toggle(t, "CRUX")
	require.Equal(t, "AURA-BLTX-CRUX", indexesOf(t, evs))
	panels := onlyRender(t, evs).Main
	e.requirePortfolio(t,
		portfolioOf(t, panels), []string{"AURA", "BLTX", "CRUX"}, t0.Unix())

	require.NotNil(t, datapagestest.ElementByID(panels, "pf-returns-AURA-BLTX-CRUX"))
	require.Nil(t, datapagestest.ElementByID(panels, "pf-returns-AURA-BLTX"))
	require.Equal(t, map[string]string{"Technology": "33%", "Energy": "33%", "Health": "33%"},
		sharesOf(t, panels))
	requireSidebar(t, mustPatch(t, evs, "sidebar"), []string{"AURA", "BLTX", "CRUX"})
	requireTabs(t, mustPatch(t, evs, "tabs"), "portfolio",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "2s",
			Range:   "15m",
			Indexes: "AURA-BLTX-CRUX",
		})

	tb.toggle(t, "AURA")
	tb.toggle(t, "BLTX")
	evs = tb.toggle(t, "CRUX")
	require.Equal(t, "none", indexesOf(t, evs))
	panels = onlyRender(t, evs).Main
	require.Nil(t, portfolioOf(t, panels))
	requireEmptyView(t, panels)
}

// TestPortfolioSelect tests all and no holdings.
func TestPortfolioSelect(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["portfolio"].href(app.ViewQuery{}),
		`{"view":"portfolio","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"DUNS"}`)
	tb.renders(t, 1)

	evs := tb.change(t, "/select/all/", "")
	panels := onlyRender(t, evs).Main
	e.requirePortfolio(t, portfolioOf(t, panels), allIndexes(), t0.Unix())
	require.ElementsMatch(t,
		market.Sectors, slices.Collect(maps.Keys(sharesOf(t, panels))))
	requireSelectButtons(t, mustPatch(t, evs, "sidebar"), false, true)

	evs = tb.change(t, "/select/none/", "")
	panels = onlyRender(t, evs).Main
	require.Nil(t, portfolioOf(t, panels))
	one(t, panels, "the empty view", class("empty"))
	requireSelectButtons(t, mustPatch(t, evs, "sidebar"), true, false)
}

// TestPortfolioPausedPage tests a shared paused portfolio URL.
func TestPortfolioPausedPage(t *testing.T) {
	e := newEnv(t)
	signals := `{"view":"portfolio","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"DUNS-HALA"}`
	e.market.Tick(t0.Add(7 * time.Second))
	live := e.openTab(t, views["portfolio"].href(app.ViewQuery{}), signals).renders(t, 1)[0]
	require.Equal(t, t0.Unix()+7, live.At)

	e.market.Tick(t0.Add(20 * time.Second))
	paused := e.openTab(t, views["portfolio"].href(app.ViewQuery{}), fmt.Sprintf(
		`{"view":"portfolio","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"DUNS-HALA","paused":%d}`,
		t0.Unix()+7,
	)).renders(t, 1)[0]
	require.Equal(t, live.At, paused.At)
	require.Equal(t, htmlOf(t, live.Main), htmlOf(t, paused.Main))
	e.requirePortfolio(t,
		portfolioOf(t, paused.Main), []string{"DUNS", "HALA"}, t0.Unix()+7)
}

// requirePortfolio checks a portfolio window through at.
func (e *env) requirePortfolio(t *testing.T, p *portfolio, symbols []string, at int64) {
	t.Helper()
	require.NotNil(t, p)
	to := at * 1000
	from := to - to%5000 - 119*5000

	times, _ := e.closes(symbols[0], from, to)
	require.Len(t, times, 120)
	opens := make([]float64, len(symbols))
	shares := make([]float64, len(symbols))
	closes := make([][]float64, len(symbols))
	values := make([]float64, len(times))
	for i, sym := range symbols {
		opens[i] = e.market.Open(indexOf(t, sym))
		shares[i] = 100_000 / float64(len(symbols)) / opens[i]
		_, closes[i] = e.closes(sym, from, to)
		require.Len(t, closes[i], len(times), sym)
	}
	for k := range times {
		for i := range symbols {
			values[k] += shares[i] * closes[i][k]
		}
	}
	// average is the return of all indexes, summed the way the server sums it.
	sums := map[int64]float64{}
	for j, in := range market.Indexes {
		ts, cs := e.closes(in.Symbol, from, to)
		for k, tm := range ts {
			sums[tm] += cs[k] / e.market.Open(j)
		}
	}
	average := func(tm int64) float64 {
		return (sums[tm]/float64(len(market.Indexes)) - 1) * 100
	}

	var rows int
	for k, tm := range times {
		sec := float64(tm / 1000)
		ret, alloc := p.Returns[rows], p.Allocation[rows]
		require.Equal(t, []float64{sec}, ret[:1], "returns row %d", rows)
		require.InDeltaSlice(t, []float64{(values[k]/100_000 - 1) * 100, average(tm)}, ret[1:], 0.006)
		require.Len(t, alloc, 1+len(symbols))
		require.Equal(t, sec, alloc[0], "allocation row %d", rows)
		var sum float64
		for i := range symbols {
			require.InDelta(t, 100*shares[i]*closes[i][k]/values[k], alloc[1+i], 0.006)
			sum += alloc[1+i]
		}
		require.InDelta(t, 100, sum, 0.05)
		rows++
	}
	require.Len(t, p.Returns, rows)
	require.Len(t, p.Allocation, rows)

	// The drawdown and the distribution span the whole window.
	require.Len(t, p.Drawdown, len(times))
	peak := values[0]
	for k, v := range values {
		peak = max(peak, v)
		require.Equal(t, float64(times[k]/1000), p.Drawdown[k][0])
		require.InDelta(t, (v/peak-1)*100, p.Drawdown[k][1], 0.006)
	}
	requireDistribution(t, values, p.Distribution)

	last := len(times) - 1
	require.Len(t, p.Profit, len(symbols))
	best, worst := 0, 0
	for i, sym := range symbols {
		require.Equal(t, sym, p.Profit[i][0])
		require.InDelta(t, shares[i]*(closes[i][last]-opens[i]), p.Profit[i][1], 0.006)
		if closes[i][last]/opens[i] > closes[best][last]/opens[best] {
			best = i
		}
		if closes[i][last]/opens[i] < closes[worst][last]/opens[worst] {
			worst = i
		}
	}

	dollars, err := strconv.Atoi(strings.NewReplacer("$", "", ",", "").Replace(p.Value))
	require.NoError(t, err, p.Value)
	require.Equal(t, int(math.Round(values[last])), dollars)
	change := (values[last]/100_000 - 1) * 100
	require.Equal(t, tile{fmt.Sprintf("%+.2f%%", change), change >= 0, ""}, p.Change)
	m := average(times[last])
	require.Equal(t, tile{fmt.Sprintf("%+.2f%%", m), m >= 0, "Equal parts of all 48 indexes"}, p.Market)
	holdingTile := func(i int) tile {
		r := (closes[i][last]/opens[i] - 1) * 100
		in := market.Indexes[indexOf(t, symbols[i])]
		return tile{fmt.Sprintf("%s %+.2f%%", in.Symbol, r), r >= 0, in.Name}
	}
	require.Equal(t, holdingTile(best), p.Best)
	require.Equal(t, holdingTile(worst), p.Worst)
}

// requireDistribution checks consecutive returns in zero-centered bins.
func requireDistribution(t *testing.T, values []float64, rows [][]float64) {
	t.Helper()
	require.Greater(t, len(rows), 1)
	require.Equal(t, 1, len(rows)%2, "bins on both sides of zero")
	side := len(rows) / 2
	width := rows[side+1][0]
	require.Zero(t, rows[side][0])
	counts := make([]float64, len(rows))
	for k := 1; k < len(values); k++ {
		r := (values[k]/values[k-1] - 1) * 100
		counts[side+int(math.Round(r/width))]++
	}
	for i, row := range rows {
		require.InDelta(t, float64(i-side)*width, row[0], 1e-9, "center of bin %d", i)
		require.Equal(t, counts[i], row[1], "count of bin %d", i)
	}
}

func (e *env) closes(sym string, from, to int64) ([]int64, []float64) {
	var times []int64
	var closes []float64
	for _, b := range e.db.OHLC(sym, from, to, 5000) {
		times = append(times, b.Time)
		closes = append(closes, b.Close)
	}
	return times, closes
}

func indexOf(t *testing.T, symbol string) int {
	t.Helper()
	for i, in := range market.Indexes {
		if in.Symbol == symbol {
			return i
		}
	}
	require.FailNow(t, "no index "+symbol)
	return 0
}

// sharesOf reads displayed sector percentages.
func sharesOf(t *testing.T, n *html.Node) map[string]string {
	t.Helper()
	shares := map[string]string{}
	for _, li := range find(one(t, n, "the donut", class("donut")), tag("li")) {
		share := datapagestest.Text(one(t, li, "the share", class("share")))
		sector := strings.TrimSpace(strings.TrimSuffix(datapagestest.Text(li), share))
		shares[sector] = share
	}
	return shares
}

// portfolioOf reads rendered panel data.
func portfolioOf(t *testing.T, n *html.Node) *portfolio {
	t.Helper()
	if len(find(n, tag("pf-chart"))) == 0 {
		return nil
	}
	p := &portfolio{}
	data := func(kind string) string {
		return attrOf(one(t,
			n, "the "+kind+" chart", tag("pf-chart"), attr("kind", kind)), "data")
	}
	for kind, v := range map[string]any{
		"lines":     &p.Returns,
		"stacked":   &p.Allocation,
		"area":      &p.Drawdown,
		"histogram": &p.Distribution,
		"bars":      &p.Profit,
	} {
		require.NoError(t, json.Unmarshal([]byte(data(kind)), v), kind)
	}
	require.Equal(t, data("lines"), data("spark"), "the spark shows the returns")
	for _, article := range find(n, tag("article"), class("tile")) {
		label := datapagestest.Text(one(t, article, "the label of a tile", tag("h2")))
		value := one(t, article, "the value of "+label, class("tile-value"))
		note := one(t, article, "the note of "+label, class("tile-note"))
		if label == "Value" {
			p.Value = datapagestest.Text(value)
			change := one(t, note, "the change of the value", tag("span"))
			p.Change = tile{Text: datapagestest.Text(change), Up: hasClass(change, "up")}
			continue
		}
		shown := tile{
			Text: datapagestest.Text(value),
			Up:   hasClass(value, "up"),
			Note: datapagestest.Text(note),
		}
		switch label {
		case "All indexes":
			p.Market = shown
		case "Best":
			p.Best = shown
		case "Worst":
			p.Worst = shown
		default:
			require.FailNow(t, "a tile "+label)
		}
	}
	return p
}
