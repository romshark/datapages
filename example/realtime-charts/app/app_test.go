// Tests the view pages, their streams and the actions over HTTP.
// The tests tick the market and publish its events themselves,
// which fixes the market time of every push.

package app_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/href"
	"github.com/romshark/datapages/example/realtime-charts/market"
	"github.com/romshark/datapages/example/realtime-charts/tsdb"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// t0 is when the market of every test starts, two seconds into a bar.
var t0 = time.Unix(1_800_000_002, 0)

const history = time.Hour

// defaults are the indexes a page without an indexes parameter shows.
var defaults = []string{"AURA", "BLTX", "CRUX", "DUNS", "ECHN", "FERN"}

var views = map[string]struct {
	label string
	href  func(q app.ViewQuery) string
}{
	"charts": {"Charts", func(q app.ViewQuery) string {
		return href.PageIndex(href.QueryPageIndex(q))
	}},
	"map": {"Map", func(q app.ViewQuery) string {
		return href.PageMap(href.QueryPageMap(q))
	}},
	"portfolio": {"Portfolio", func(q app.ViewQuery) string {
		return href.PagePortfolio(href.QueryPagePortfolio(q))
	}},
}

// TestPage tests the chart type, the charts, the sidebar,
// the tabs and the settings a load of the chart page shows.
func TestPage(t *testing.T) {
	e := newEnv(t)
	first := t0.Add(-history).Unix()
	for name, tc := range map[string]struct {
		typ     string
		query   string
		indexes []string
		refresh string
		rng     string // "15m" when empty
		paused  int64
		sync    bool
		layout  string // "grid" when empty
	}{
		"defaults": {
			typ:     "candles",
			indexes: defaults,
			refresh: "1s",
		},
		"rows": {
			typ:     "line",
			query:   "type=line&layout=rows",
			indexes: defaults, refresh: "1s",
			layout: "rows",
		},
		"unknown layout": {
			typ:     "candles",
			query:   "layout=columns",
			indexes: defaults,
			refresh: "1s",
		},
		"synced": {
			typ:     "line",
			query:   "type=line&sync=true",
			indexes: defaults,
			refresh: "1s", sync: true,
		},
		"from the query": {
			typ:     "bars",
			query:   "type=bars&refresh=5s&range=1h&indexes=GLWT-DUNS",
			indexes: []string{"DUNS", "GLWT"},
			refresh: "5s", rng: "1h",
		},
		"shortest range": {
			typ:     "line",
			query:   "type=line&range=5m",
			indexes: defaults,
			refresh: "1s", rng: "5m",
		},
		"unknown range": {
			typ:     "area",
			query:   "type=area&range=2m",
			indexes: defaults,
			refresh: "1s",
		},
		"range longer than the history": {
			typ:     "area",
			query:   "type=area&range=3h",
			indexes: defaults,
			refresh: "1s",
		},
		"unknown and repeated indexes": {
			typ:     "line",
			query:   "type=line&indexes=GLWT-XXXX-GLWT",
			indexes: []string{"GLWT"},
			refresh: "1s",
		},
		"no indexes": {typ: "area",
			query:   "type=area&indexes=none",
			refresh: "1s"},
		"unknown refresh interval": {
			typ:     "area",
			query:   "type=area&refresh=7s",
			indexes: defaults,
			refresh: "1s",
		},
		"unknown chart type": {
			typ:     "candles",
			query:   "type=pie",
			indexes: defaults,
			refresh: "1s",
		},
		"paused": {
			typ:     "baseline",
			query:   fmt.Sprintf("type=baseline&paused=%d", t0.Unix()-60),
			indexes: defaults,
			refresh: "1s",
			paused:  t0.Unix() - 60,
		},
		"paused before the history": {
			typ:     "histogram",
			query:   fmt.Sprintf("type=histogram&paused=%d", first-600),
			indexes: defaults,
			refresh: "1s",
			paused:  first,
		},
		"paused after the last tick": {
			typ:     "candles",
			query:   fmt.Sprintf("paused=%d", t0.Unix()+600),
			indexes: defaults,
			refresh: "1s",
			paused:  t0.Unix(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			rng := cmp.Or(tc.rng, "15m")
			lay := cmp.Or(tc.layout, "grid")
			path := views["charts"].href(app.ViewQuery{})
			if tc.query != "" {
				path += "?" + tc.query
			}
			resp := e.get(t, path)
			require.Equal(t, http.StatusOK, resp.Status)
			doc := resp.Document()

			charts := find(doc, tag("rt-chart"))
			require.Len(t, charts, len(tc.indexes))
			for _, c := range charts {
				require.Equal(t, tc.typ, attrOf(c, "type"))
				require.Equal(t, strconv.FormatInt(barOf[rng]/1000, 10), attrOf(c, "bar"))
				require.True(t,
					strings.HasPrefix(attrOf(c, "data-effect"), "el.sync = $sync; el.bars = "),
					attrOf(c, "data-effect"))
			}
			for _, sym := range tc.indexes {
				resp.ElementByID(t, "chart-"+sym+"-"+tc.typ+"-"+rng)
			}
			require.Len(t, find(doc, class("deactivate"), attr("title", "Deactivate the chart")),
				len(tc.indexes))
			if len(tc.indexes) == 0 {
				one(t, doc, "the empty view", class("empty"))
			}
			sidebar := resp.ElementByID(t, "sidebar")
			requireSidebar(t, sidebar, tc.indexes)
			requireSelectButtons(t, sidebar,
				len(tc.indexes) < len(market.Indexes), len(tc.indexes) > 0)

			q := app.ViewQuery{
				Type: tc.typ, Refresh: tc.refresh, Range: rng,
				Indexes: indexesParam(tc.indexes), Paused: tc.paused, Sync: tc.sync, Layout: lay,
			}
			require.Equal(t, map[string]any{
				"view": "charts", "type": tc.typ, "refresh": tc.refresh, "range": rng,
				"indexes": q.Indexes, "paused": float64(tc.paused), "sync": tc.sync,
				"layout": lay, "pushed": float64(0),
			}, resp.Signals(t))
			refresh := selectOf(t, doc, "refresh")
			requireSelected(t, refresh, tc.refresh)
			requireSelected(t, selectOf(t, doc, "type"), tc.typ)
			rangeSelect := selectOf(t, doc, "range")
			requireSelected(t, rangeSelect, rng)
			require.Equal(t, "Time range", attrOf(rangeSelect, "title"))
			// A paused page can show another range up to the second it is paused at.
			_, disabled := datapagestest.Attr(rangeSelect, "disabled")
			require.False(t, disabled, "the time range select is disabled")

			live := one(t, doc, "the live button", class("live"))
			title := "Pause updates"
			if tc.paused != 0 {
				title = "Resume updates"
			}
			require.Equal(t, title, attrOf(live, "title"))
			require.Equal(t, tc.paused != 0, hasClass(live, "paused"))
			_, disabled = datapagestest.Attr(refresh, "disabled")
			require.Equal(t, tc.paused != 0, disabled, "the refresh select is disabled")

			main := one(t, doc, "main", tag("main"))
			toolbar := one(t, main, "the toolbar", class("toolbar"))
			one(t, toolbar, "the chart type select", tag("select"), attr("data-bind", "type"))
			one(t, toolbar, "the time range select", tag("select"), attr("data-bind", "range"))
			sync := one(t, toolbar, "the sync button", class("sync"))
			require.Equal(t, strconv.FormatBool(tc.sync), attrOf(sync, "aria-pressed"))
			require.Equal(t, "Sync charts", attrOf(sync, "title"))
			kids := children(sync)
			require.Equal(t, []string{"svg", "span"}, tagsOf(kids),
				"the icon, then the name")
			require.Equal(t, "Sync", datapagestest.Text(kids[1]))
			for _, l := range []string{"rows", "grid"} {
				segment := one(t, toolbar, "the "+l+" layout button", class("segment"),
					attrContains("data-on:click", `$layout = "`+l+`"`))
				require.Equal(t,
					strconv.FormatBool(l == lay), attrOf(segment, "aria-pressed"), l)
				require.Equal(t, "svg", children(segment)[0].Data, l)
			}
			require.Equal(t, lay == "rows", attrOf(main, "class") == "rows")
			// A hidden tab shown again reconnects its stream instead of reloading,
			// which would end a pause.
			require.NotContains(t, resp.Body, "visibilitychange")
			requireTabs(t, resp.ElementByID(t, "tabs"), "charts", q)
		})
	}
}

// TestPageRejectsMalformedPause tests that a paused parameter
// that is not a number is a bad request.
func TestPageRejectsMalformedPause(t *testing.T) {
	resp := newEnv(t).get(t, views["charts"].href(app.ViewQuery{})+"?type=line&paused=soon")
	require.Equal(t, http.StatusBadRequest, resp.Status)
}

// TestStreamOpen tests that a new stream receives the last window of bars of
// the indexes its tab shows, as the points its chart type draws.
func TestStreamOpen(t *testing.T) {
	for name, tc := range map[string]struct {
		signals string
		indexes []string
		shape   string
		rng     string // "15m" when empty
	}{
		"candles": {
			signals: `{"view":"charts","type":"candles","layout":"grid","refresh":"2s","range":"15m","indexes":"GLWT-AURA"}`,
			indexes: []string{"AURA", "GLWT"}, shape: "ohlc",
		},
		"bars": {
			signals: `{"view":"charts","type":"bars","layout":"grid","refresh":"2s","range":"15m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "ohlc",
		},
		"line": {
			signals: `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"15m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"area": {
			signals: `{"view":"charts","type":"area","layout":"grid","refresh":"2s","range":"15m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"baseline": {
			signals: `{"view":"charts","type":"baseline","layout":"grid","refresh":"2s","range":"15m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"histogram": {
			signals: `{"view":"charts","type":"histogram","layout":"grid","refresh":"2s","range":"15m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "change",
		},
		"last 5 minutes": {
			signals: `{"view":"charts","type":"candles","layout":"grid","refresh":"2s","range":"5m","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "ohlc", rng: "5m",
		},
		"last hour": {
			signals: `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"1h","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close", rng: "1h",
		},
		"unknown settings": {
			signals: `{"view":"pie","type":"pie","layout":"grid","refresh":"7s","range":"2m"}`,
			indexes: defaults, shape: "ohlc",
		},
		"no indexes": {
			signals: `{"view":"charts","type":"line","layout":"grid","refresh":"1s","range":"15m","indexes":"none"}`,
			shape:   "close",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			p := e.openTab(t, "/", tc.signals).pushes(t, 1)[0]
			require.Equal(t, t0.Unix(), p.Pushed)
			require.Equal(t, tc.indexes, sortedKeys(p.Feed))
			for _, sym := range tc.indexes {
				e.requireWindowOf(t, sym, cmp.Or(tc.rng, "15m"), t0.Unix(), p.Feed[sym], tc.shape)
			}
		})
	}
}

// TestPushInterval tests that a stream receives the bars that
// changed since its previous push, once per refresh interval.
func TestPushInterval(t *testing.T) {
	for name, interval := range map[string]int{"1s": 1, "2s": 2, "5s": 5, "30s": 30} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tb := e.openTab(t, "/",
				`{"view":"charts","type":"candles","layout":"grid","refresh":"`+name+`","indexes":"AURA-GLWT-VRGE"}`)
			tb.pushes(t, 1)

			for push := 1; push <= 2; push++ {
				// Waiting for each push keeps the market from moving on before the stream
				// handled the ticks of the interval.
				for s := (push-1)*interval + 1; s <= push*interval; s++ {
					e.tick(t, s)
				}
				p := tb.pushes(t, push+1)[push]
				now := t0.Unix() + int64(push*interval)
				require.Equal(t, now, p.Pushed, "push %d", push)
				require.Equal(t, []string{"AURA", "GLWT", "VRGE"}, sortedKeys(p.Feed))
				for sym, c := range p.Feed {
					e.requireSince(t, sym, now-int64(interval), now, c, "ohlc")
				}
			}
			require.Len(t, tb.pushes(t, 0), 3, "pushes between the intervals")
		})
	}
}

// TestHiddenIndexEvents tests that the events of indexes
// a tab does not show push nothing to it.
func TestHiddenIndexEvents(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"DUNS"}`)
	tb.pushes(t, 1)

	e.market.Tick(t0.Add(time.Second))
	e.publish(t, "AURA")
	e.publish(t, "VRGE")
	// The broker delivers the events of a stream in the order they were published,
	// and a settings change pushes nothing: once it is rendered, the stream handled
	// the events of the hidden indexes.
	tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`)
	require.Len(t, tb.pushes(t, 1), 1, "pushes for hidden indexes")

	e.publish(t, "DUNS")
	p := tb.pushes(t, 2)[1]
	require.Equal(t, t0.Unix()+1, p.Pushed)
	require.Equal(t, []string{"DUNS"}, sortedKeys(p.Feed))
	e.requireSince(t, "DUNS", t0.Unix(), t0.Unix()+1, p.Feed["DUNS"], "ohlc")
}

// TestToggle tests that a toggle shows or hides the chart of an index: the charts it
// adds get their window of bars, the charts it removes lose their signals,
// and the sidebar, the grid, the tabs and the URL follow.
func TestToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)

	evs := tb.toggle(t, "CRUX")
	p := onlyPatch(t, evs)
	require.Equal(t, "AURA-BLTX-CRUX", p.Indexes)
	require.Equal(t, []string{"CRUX"}, sortedKeys(p.Feed))
	e.requireWindow(t, "CRUX", t0.Unix(), p.Feed["CRUX"], "close")
	grid := mustPatch(t, evs, "grid")
	require.Len(t, find(grid, tag("rt-chart")), 3)
	require.Len(t, find(grid, tag("rt-chart"), attr("type", "line")), 3)
	requireSidebar(t, mustPatch(t, evs, "sidebar"), []string{"AURA", "BLTX", "CRUX"})
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts",
		app.ViewQuery{
			Type:    "line",
			Layout:  "grid",
			Refresh: "2s",
			Range:   "15m",
			Indexes: "AURA-BLTX-CRUX",
		})

	e.tick(t, 1)
	e.tick(t, 2)
	require.Equal(t,
		[]string{"AURA", "BLTX", "CRUX"}, sortedKeys(tb.pushes(t, 2)[1].Feed))

	evs = tb.toggle(t, "AURA")
	p = onlyPatch(t, evs)
	require.Equal(t, "BLTX-CRUX", p.Indexes)
	require.Equal(t, map[string]*feed{"AURA": nil}, p.Feed)
	require.Len(t, find(mustPatch(t, evs, "grid"), tag("rt-chart")), 2)

	tb.toggle(t, "BLTX")
	evs = tb.toggle(t, "CRUX")
	require.Equal(t, "none", onlyPatch(t, evs).Indexes)
	requireEmptyView(t, mustPatch(t, evs, "grid"))

	resp := tb.post(t, "/toggle/XXXX/", "")
	require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
		"Something went wrong.")
}

// TestSelect tests that the buttons of the sidebar show the charts of all indexes and
// hide all of them, and are disabled when they would change nothing.
func TestSelect(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)
	all := allIndexes()

	evs := tb.change(t, "/select/all/", "")
	p := onlyPatch(t, evs)
	require.Equal(t, strings.Join(all, "-"), p.Indexes)
	require.Equal(t, slices.Sorted(slices.Values(all[2:])), sortedKeys(p.Feed))
	for sym, c := range p.Feed {
		e.requireWindow(t, sym, t0.Unix(), c, "close")
	}
	require.Len(t, find(mustPatch(t, evs, "grid"), tag("rt-chart")), len(all))
	sidebar := mustPatch(t, evs, "sidebar")
	requireSidebar(t, sidebar, all)
	requireSelectButtons(t, sidebar, false, true)
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts",
		app.ViewQuery{
			Type:    "line",
			Layout:  "grid",
			Refresh: "2s",
			Range:   "15m",
			Indexes: strings.Join(all, "-"),
		})

	evs = tb.change(t, "/select/none/", "")
	p = onlyPatch(t, evs)
	require.Equal(t, "none", p.Indexes)
	require.Len(t, p.Feed, len(all))
	for sym, c := range p.Feed {
		require.Nil(t, c, sym)
	}
	one(t, mustPatch(t, evs, "grid"), "the empty view", class("empty"))
	sidebar = mustPatch(t, evs, "sidebar")
	requireSidebar(t, sidebar, nil)
	requireSelectButtons(t, sidebar, true, false)

	resp := tb.post(t, "/select/some/", "")
	require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
		"Something went wrong.")
}

// TestTimeRange tests that a time range change redraws every chart with the bars of the
// new range, up to the second a paused tab is paused at, replaces the charts of the grid
// with charts of that range and links the tabs with it. The pushes after it carry bars
// of the length of that range.
func TestTimeRange(t *testing.T) {
	for name, tc := range map[string]struct {
		rng     string
		history time.Duration // an hour when zero
		paused  int64
	}{
		"last 5 minutes":  {rng: "5m"},
		"last 30 minutes": {rng: "30m"},
		"last hour":       {rng: "1h"},
		"last 3 hours":    {rng: "3h", history: 3 * time.Hour},
		"paused":          {rng: "30m", paused: t0.Unix() - 90},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnvWith(t, cmp.Or(tc.history, history))
			tb := e.openTab(t, "/", fmt.Sprintf(
				`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-BLTX","paused":%d}`,
				tc.paused,
			))
			// A paused page receives the bars of its second first.
			tb.pushes(t, 1)

			evs := tb.settings(t, fmt.Sprintf(
				`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"%s","paused":%d}`, tc.rng, tc.paused,
			))
			at := cmp.Or(tc.paused, t0.Unix())
			p := onlyPatch(t, evs)
			require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(p.Feed))
			for sym, c := range p.Feed {
				e.requireWindowOf(t, sym, tc.rng, at, c, "ohlc")
			}
			grid := mustPatch(t, evs, "grid")
			for _, sym := range []string{"AURA", "BLTX"} {
				require.NotNil(t,
					datapagestest.ElementByID(grid, "chart-"+sym+"-candles-"+tc.rng), sym)
			}
			require.Len(t, find(grid, tag("rt-chart"),
				attr("bar", strconv.FormatInt(barOf[tc.rng]/1000, 10))), 2)
			require.Nil(t, patched(evs, "sidebar"), "the sidebar did not change")
			requireTabs(t, mustPatch(t, evs, "tabs"), "charts", app.ViewQuery{
				Type:    "candles",
				Layout:  "grid",
				Refresh: "1s",
				Range:   tc.rng,
				Indexes: "AURA-BLTX",
				Paused:  tc.paused,
			})
			if tc.paused != 0 {
				return
			}

			e.tick(t, 1)
			p = tb.pushes(t, 2)[1]
			require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(p.Feed))
			for sym, c := range p.Feed {
				e.requireSinceOf(t, sym, tc.rng, t0.Unix(), t0.Unix()+1, c, "ohlc")
			}
		})
	}
}

// TestTimeRangeWithoutCharts tests that a time range change on the market map and the
// portfolio page, which have no time range, sends only the tabs linked with it,
// and that their pages have no chart type or time range select.
func TestTimeRangeWithoutCharts(t *testing.T) {
	for _, view := range []string{"map", "portfolio"} {
		t.Run(view, func(t *testing.T) {
			e := newEnv(t)
			path := views[view].href(app.ViewQuery{})
			doc := e.get(t, path).Document()
			require.Empty(t, find(doc, attr("data-bind", "type")))
			require.Empty(t, find(doc, attr("data-bind", "range")))

			tb := e.openTab(t, path, `{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
			tb.pushes(t, 1)
			evs := tb.settings(t, `{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"1h"}`)
			p := onlyPatch(t, evs)
			require.Nil(t, p.Feed)
			require.Nil(t, p.Map)
			require.Nil(t, p.Portfolio)
			require.Nil(t, patched(evs, "market"))
			require.Nil(t, patched(evs, "portfolio"))
			requireTabs(t, mustPatch(t, evs, "tabs"), view,
				app.ViewQuery{
					Type:    "candles",
					Layout:  "grid",
					Refresh: "1s",
					Range:   "1h",
					Indexes: "AURA",
				})
		})
	}
}

// TestChartType tests that a chart type change redraws the charts of the tab as the new
// series, starting over with a window of bars in its points, and that the tabs link it.
// The market map and the portfolio page redraw nothing: they keep the type for the tab
// of the charts.
func TestChartType(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)

	evs := tb.settings(t, `{"view":"charts","type":"histogram","layout":"grid","refresh":"1s","range":"15m"}`)
	p := onlyPatch(t, evs)
	require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(p.Feed))
	for sym, c := range p.Feed {
		e.requireWindow(t, sym, t0.Unix(), c, "change")
	}
	grid := mustPatch(t, evs, "grid")
	require.Len(t, find(grid, tag("rt-chart"), attr("type", "histogram")), 2)
	for _, sym := range []string{"AURA", "BLTX"} {
		require.NotNil(t,
			datapagestest.ElementByID(grid, "chart-"+sym+"-histogram-15m"), sym)
	}
	require.Nil(t, patched(evs, "sidebar"), "the sidebar did not change")
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts", app.ViewQuery{
		Type:    "histogram",
		Layout:  "grid",
		Refresh: "1s",
		Range:   "15m",
		Indexes: "AURA-BLTX",
	})

	e.tick(t, 1)
	p = tb.pushes(t, 2)[1]
	for sym, c := range p.Feed {
		e.requireSince(t, sym, t0.Unix(), t0.Unix()+1, c, "change")
	}

	for _, view := range []string{"map", "portfolio"} {
		other := e.openTab(t, views[view].href(app.ViewQuery{}),
			`{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
		other.pushes(t, 1)
		evs := other.settings(t, `{"view":"`+view+`","type":"bars","layout":"grid","refresh":"1s","range":"15m"}`)
		require.Nil(t, onlyPatch(t, evs).Feed, view)
		require.Nil(t, patched(evs, "market"), view)
		require.Nil(t, patched(evs, "portfolio"), view)
		requireTabs(t, mustPatch(t, evs, "tabs"), view,
			app.ViewQuery{Type: "bars", Layout: "grid", Refresh: "1s", Range: "15m", Indexes: "AURA"})
	}
}

// TestTimeRanges tests that the time range select offers the ranges the history spans
// with their labels, and that a history shorter than the default range makes
// the longest of them the default of a page and a stream.
func TestTimeRanges(t *testing.T) {
	for name, tc := range map[string]struct {
		history time.Duration
		labels  []string
		dflt    string
	}{
		"ten minutes": {
			history: 10 * time.Minute,
			labels:  []string{"Last 5 minutes"},
			dflt:    "5m",
		},
		"an hour": {
			history: time.Hour,
			dflt:    "15m",
			labels: []string{
				"Last 5 minutes", "Last 15 minutes", "Last 30 minutes", "Last 1 hour",
			},
		},
		"three hours": {
			history: 3 * time.Hour,
			dflt:    "15m",
			labels: []string{
				"Last 5 minutes", "Last 15 minutes", "Last 30 minutes", "Last 1 hour",
				"Last 3 hours",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnvWith(t, tc.history)
			sel := selectOf(t, e.get(t, views["charts"].href(app.ViewQuery{})).Document(), "range")
			var labels []string
			for _, opt := range find(sel, tag("option")) {
				labels = append(labels, datapagestest.Text(opt))
			}
			require.Equal(t, tc.labels, labels)
			requireSelected(t, sel, tc.dflt)

			p := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","indexes":"AURA"}`).pushes(t, 1)[0]
			e.requireWindowOf(t, "AURA", tc.dflt, t0.Unix(), p.Feed["AURA"], "ohlc")
		})
	}
}

// TestPause tests that a paused tab receives no push while the market moves,
// and every bar that changed during the pause once it resumes.
func TestPause(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)

	paused := fmt.Sprintf(`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","paused":%d}`, t0.Unix())
	tb.settings(t, paused)
	for s := 1; s <= 5; s++ {
		e.tick(t, s)
	}
	// The settings change after the ticks arrives after them: once it is rendered,
	// the stream handled every tick during the pause.
	tb.settings(t, paused)
	require.Len(t, tb.pushes(t, 1), 1, "pushes during the pause")

	pushes := pushesOf(t, tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`))
	require.Len(t, pushes, 1)
	require.Equal(t, t0.Unix()+5, pushes[0].Pushed)
	for sym, c := range pushes[0].Feed {
		e.requireSince(t, sym, t0.Unix(), t0.Unix()+5, c, "ohlc")
	}

	e.tick(t, 6)
	require.Equal(t, t0.Unix()+6, tb.pushes(t, 3)[2].Pushed)
}

// TestPauseLongerThanTheWindow tests that a tab resuming after a pause longer than the
// first window receives every bar of the pause.
func TestPauseLongerThanTheWindow(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"line","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	tb.pushes(t, 1)
	tb.settings(t, fmt.Sprintf(`{"view":"charts","type":"line","layout":"grid","refresh":"1s","range":"15m","paused":%d}`, t0.Unix()))

	// The market moves without events, which the paused tab would ignore.
	e.market.Tick(t0.Add(30 * time.Minute))
	pushes := pushesOf(t, tb.settings(t, `{"view":"charts","type":"line","layout":"grid","refresh":"1s","range":"15m"}`))
	require.Len(t, pushes, 1)
	now := t0.Add(30 * time.Minute).Unix()
	require.Greater(t, len(pushes[0].Feed["AURA"].Bars), 120)
	e.requireSince(t, "AURA", t0.Unix(), now, pushes[0].Feed["AURA"], "close")
}

// TestPausedPage tests that a page opened paused at a second shows what a live page
// showed at that second, the bar that was forming included.
func TestPausedPage(t *testing.T) {
	e := newEnv(t)
	signals := `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-GLWT"}`
	e.market.Tick(t0.Add(7 * time.Second))
	live := e.openTab(t, "/", signals).pushes(t, 1)[0]
	require.Equal(t, t0.Unix()+7, live.Pushed)

	// The bar forming at t0+7 closes, and the next one starts.
	e.market.Tick(t0.Add(20 * time.Second))
	pausedAt := fmt.Sprintf(`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-GLWT","paused":%d}`,
		t0.Unix()+7)
	tb := e.openTab(t, "/", pausedAt)
	p := tb.pushes(t, 1)[0]
	require.Equal(t, live, p)

	e.tick(t, 21)
	pushes := pushesOf(t, tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`))
	require.Len(t, pushes, 1)
	for sym, c := range pushes[0].Feed {
		e.requireSince(t, sym, t0.Unix()+7, t0.Unix()+21, c, "ohlc")
	}
}

// TestStreamOpenResumes tests that a stream reconnecting with the second of
// the last push its tab received gets the bars updated since,
// and nothing until its tab resumes when it is paused.
func TestStreamOpenResumes(t *testing.T) {
	for name, tc := range map[string]struct {
		pushed int64
		paused bool
		since  int64 // the second the first push continues from, zero for the window
	}{
		"live": {
			pushed: t0.Unix() - 12, since: t0.Unix() - 12,
		},
		"from before the history": {
			pushed: t0.Unix() - 7200, since: t0.Unix() - 7200,
		},
		"after the last tick": {
			pushed: t0.Unix() + 60,
		},
		"paused": {
			pushed: t0.Unix() - 12, paused: true, since: t0.Unix() - 12,
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			paused := int64(0)
			if tc.paused {
				paused = tc.pushed
			}
			tb := e.openTab(t, "/", fmt.Sprintf(
				`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA","paused":%d,"pushed":%d}`,
				paused, tc.pushed,
			))
			if tc.paused {
				tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`)
			}
			// A paused tab gets only the push of its resume: StreamOpen pushed nothing.
			pushes := tb.pushes(t, 1)
			require.Len(t, pushes, 1)
			require.Equal(t, t0.Unix(), pushes[0].Pushed)
			if tc.since == 0 {
				e.requireWindow(t, "AURA", t0.Unix(), pushes[0].Feed["AURA"], "ohlc")
			} else {
				e.requireSince(t, "AURA", tc.since, t0.Unix(), pushes[0].Feed["AURA"], "ohlc")
			}
		})
	}
}

// TestSync tests that the sync button turns the sync of a tab on and off without
// redrawing its charts and that the links of the tabs carry it, also after
// a stream opens with it. The market map and the portfolio page have no toolbar,
// but keep the signal for the links of their tabs.
func TestSync(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	tb.pushes(t, 1)

	evs := tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","sync":true}`)
	p := onlyPatch(t, evs)
	require.Nil(t, p.Feed)
	require.Nil(t, patched(evs, "grid"))
	require.Nil(t, patched(evs, "sidebar"))
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "1s",
			Range:   "15m",
			Indexes: "AURA",
			Sync:    true,
		})

	evs = tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","sync":false}`)
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "1s",
			Range:   "15m",
			Indexes: "AURA",
		})

	synced := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA","sync":true}`)
	synced.pushes(t, 1)
	requireTabs(t, mustPatch(t, synced.toggle(t, "BLTX"), "tabs"), "charts",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "1s",
			Range:   "15m",
			Indexes: "AURA-BLTX",
			Sync:    true,
		})

	for _, v := range []string{"portfolio", "map"} {
		resp := e.get(t, views[v].href(app.ViewQuery{Sync: true}))
		require.Equal(t, true, resp.Signals(t)["sync"], v)
		require.Empty(t, find(resp.Document(), class("toolbar")), v)
		require.Empty(t, find(resp.Document(), attrContains("data-effect", "el.sync")), v)
	}
}

// TestLayout tests that the layout buttons change the layout of a tab without redrawing
// its charts, that the links of the tabs carry it, also after a stream opens with it,
// and that the market map and the portfolio page, which have no layout buttons,
// keep it for the tab of the charts.
func TestLayout(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	tb.pushes(t, 1)

	evs := tb.settings(t, `{"view":"charts","type":"candles","layout":"rows","refresh":"1s","range":"15m"}`)
	require.Nil(t, onlyPatch(t, evs).Feed)
	require.Nil(t, patched(evs, "grid"))
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts", app.ViewQuery{
		Type:    "candles",
		Layout:  "rows",
		Refresh: "1s",
		Range:   "15m",
		Indexes: "AURA",
	})

	rows := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"rows","refresh":"1s","range":"15m","indexes":"AURA"}`)
	rows.pushes(t, 1)
	requireTabs(t, mustPatch(t, rows.toggle(t, "BLTX"), "tabs"), "charts", app.ViewQuery{
		Type:    "candles",
		Layout:  "rows",
		Refresh: "1s",
		Range:   "15m",
		Indexes: "AURA-BLTX",
	})

	for _, view := range []string{"map", "portfolio"} {
		resp := e.get(t, views[view].href(app.ViewQuery{Layout: "rows"}))
		require.Equal(t, "rows", resp.Signals(t)["layout"], view)
		require.Empty(t, find(resp.Document(), class("segment")), view)
	}
}

// TestSettingsReachOnlyTheirTab tests that a settings
// change leaves another tab of the same view alone.
func TestSettingsReachOnlyTheirTab(t *testing.T) {
	e := newEnv(t)
	changing := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	other := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	changing.pushes(t, 1)
	other.pushes(t, 1)

	changing.toggle(t, "BLTX")

	// Had the other tab received the settings event,
	// it would have arrived before the tick.
	e.tick(t, 1)
	other.pushes(t, 2)
	for _, ev := range other.st.Events() {
		require.Equal(t, datapagestest.PatchSignals, ev.Type)
	}
	require.Equal(t, []string{"AURA"}, sortedKeys(other.pushes(t, 2)[1].Feed))
	require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(changing.pushes(t, 2)[1].Feed))
}

// TestSettingsRejectsUnknownValues tests that the settings action rejects a chart type,
// a layout, a refresh interval or a time range the page doesn't offer and
// keeps the tab's settings.
func TestSettingsRejectsUnknownValues(t *testing.T) {
	for name, signals := range map[string]string{
		"chart type":       `{"view":"charts","type":"pie","layout":"grid","refresh":"1s","range":"15m"}`,
		"no chart type":    `{"view":"charts","refresh":"1s","range":"15m"}`,
		"layout":           `{"view":"charts","type":"candles","layout":"columns","refresh":"1s","range":"15m"}`,
		"no layout":        `{"view":"charts","type":"candles","refresh":"1s","range":"15m"}`,
		"refresh interval": `{"view":"charts","type":"candles","layout":"grid","refresh":"3s","range":"15m"}`,
		"time range":       `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"2m"}`,
		"no time range":    `{"view":"charts","type":"candles","layout":"grid","refresh":"1s"}`,
		"beyond history":   `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"3h"}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
			tb.pushes(t, 1)

			resp := tb.post(t, "/settings/", signals)
			require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
				"The server rejected these settings.")

			e.tick(t, 1)
			p := tb.pushes(t, 2)[1]
			require.Equal(t, t0.Unix()+1, p.Pushed)
			e.requireSince(t, "AURA", t0.Unix(), t0.Unix()+1, p.Feed["AURA"], "ohlc")
			for _, ev := range tb.st.Events() {
				require.Equal(t, datapagestest.PatchSignals, ev.Type)
			}
		})
	}
}

type env struct {
	srv    *datapagestest.Server
	v      *datapagestest.Visitor
	market *market.Market
	db     *tsdb.DB
	broker messaging.Broker
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, history)
}

// newEnvWith returns an env whose database holds the given history.
func newEnvWith(t *testing.T, history time.Duration) *env {
	t.Helper()
	db := tsdb.New(history)
	m := market.New(db, t0, history)
	// The tests publish ticks faster than a stream handles them.
	// A buffer larger than the events of any test keeps the broker from dropping one.
	broker := inmem.New(4096)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.New(db, m), broker,
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithDatastarJS(assets.Path("datastar-rocket.js")))
	require.NoError(t, err)
	srv := datapagestest.New(t, s)
	return &env{srv: srv, v: srv.Visitor(t), market: m, db: db, broker: broker}
}

func (e *env) get(t *testing.T, path string) *datapagestest.Response {
	t.Helper()
	return e.v.Get(t, path)
}

// tick moves the market to the given number of seconds after t0 and publishes the events
// the market loop of cmd/server publishes.
func (e *env) tick(t *testing.T, seconds int) {
	t.Helper()
	e.market.Tick(t0.Add(time.Duration(seconds) * time.Second))
	for _, in := range market.Indexes {
		e.publish(t, in.Symbol)
	}
}

func (e *env) publish(t *testing.T, symbol string) {
	t.Helper()
	datapagestest.Publish(t, e.broker,
		datapagestest.Subject(datapagesgen.EvPrefixIndexTicked, symbol),
		app.EventIndexTicked{Index: datapages.Subject(symbol)})
}

// The bars of a time range are the shortest of 5s, 10s, 15s, 30s, 1m, 2m, 5m, 10m and 15m
// that keep the range within 120 bars, here in milliseconds.
var barOf = map[string]int64{
	"5m": 5_000, "15m": 10_000, "30m": 15_000, "1h": 30_000,
	"3h": 120_000, "6h": 300_000, "12h": 600_000, "24h": 900_000,
}

// requireWindow checks that c carries the bars of the default time range of the last 15
// minutes of sym up to the second at, as points of the given shape,
// and the price at that second.
func (e *env) requireWindow(t *testing.T, sym string, at int64, c *feed, shape string) {
	t.Helper()
	e.requireWindowOf(t, sym, "15m", at, c, shape)
}

// requireWindowOf checks what requireWindow checks for the time range rng.
func (e *env) requireWindowOf(
	t *testing.T, sym, rng string, at int64, c *feed, shape string,
) {
	t.Helper()
	d, err := time.ParseDuration(rng)
	require.NoError(t, err)
	step := barOf[rng]
	to := at * 1000
	e.requireChart(t, sym, step, to-to%step-(d.Milliseconds()/step-1)*step, to, c, shape)
}

// requireSince checks that c carries the bars of the default time range of sym that
// changed after the second pushed, up to the second now, and the price at now.
func (e *env) requireSince(
	t *testing.T, sym string, pushed, now int64, c *feed, shape string,
) {
	t.Helper()
	e.requireSinceOf(t, sym, "15m", pushed, now, c, shape)
}

// requireSinceOf checks what requireSince checks for the time range rng.
func (e *env) requireSinceOf(
	t *testing.T, sym, rng string, pushed, now int64, c *feed, shape string,
) {
	t.Helper()
	step := barOf[rng]
	from := pushed * 1000
	e.requireChart(t, sym, step, from-from%step, now*1000, c, shape)
}

func (e *env) requireChart(
	t *testing.T, sym string, step, from, to int64, c *feed, shape string,
) {
	t.Helper()
	require.NotNil(t, c, sym)
	buckets := e.db.OHLC(sym, from, to, step)
	require.NotEmpty(t, buckets, sym)
	require.Len(t, c.Bars, len(buckets), sym)
	for i, b := range buckets {
		var want []float64
		switch shape {
		case "ohlc":
			want = []float64{b.Open, b.High, b.Low, b.Close}
		case "close":
			want = []float64{b.Close}
		case "change":
			want = []float64{b.Close - b.Open}
		}
		p := c.Bars[i]
		require.Len(t, p, 1+len(want), "%s bar %d", sym, i)
		require.Equal(t, float64(b.Time/1000), p[0], "%s bar %d", sym, i)
		require.InDeltaSlice(t, want, p[1:], 0.011, "%s bar %d", sym, i)
	}
	last, ok := e.db.At(sym, to)
	require.True(t, ok, sym)
	require.Equal(t, fmt.Sprintf("%.2f", last.Value), c.Price, sym)
}

// tab is a loaded page and its connected stream.
type tab struct {
	env *env
	tab *datapagestest.Tab
	st  *datapagestest.Stream
}

// openTab loads the page at path and connects its stream with signals,
// the signals Datastar sends for it.
func (e *env) openTab(t *testing.T, path, signals string) *tab {
	t.Helper()
	page := e.v.Load(t, path)
	require.NotEmpty(t, page.Instance())
	return &tab{env: e, tab: page, st: page.Connect(t, json.RawMessage(signals))}
}

// pushes waits until the stream delivered at least
// n pushes of bars and returns all of them.
func (tb *tab) pushes(t *testing.T, n int) []patch {
	t.Helper()
	var got []patch
	tb.env.srv.WaitFor(t, fmt.Sprintf("%d pushes", n), func() bool {
		got = pushesOf(t, tb.st.Events())
		return len(got) >= n
	})
	return got
}

func (tb *tab) settings(t *testing.T, signals string) []datapagestest.Event {
	t.Helper()
	return tb.change(t, "/settings/", signals)
}

func (tb *tab) toggle(t *testing.T, symbol string) []datapagestest.Event {
	t.Helper()
	return tb.change(t, "/toggle/"+symbol+"/", "")
}

// change sends an action that dispatches a settings change and returns
// the events the stream delivered for it. The action itself answers nothing.
func (tb *tab) change(t *testing.T, path, signals string) []datapagestest.Event {
	t.Helper()
	skip := len(tb.st.Events())
	require.Empty(t, tb.post(t, path, signals))
	var evs []datapagestest.Event
	// [app.Dashboard.OnSettingsChanged] patches the error message last.
	tb.env.srv.WaitFor(t, "the settings change", func() bool {
		evs = tb.st.Events()[skip:]
		return patched(evs, "error") != nil
	})
	return evs
}

// post sends an action with signals, "" for none, and returns the events of its response.
func (tb *tab) post(t *testing.T, path, signals string) []datapagestest.Event {
	t.Helper()
	var body any
	if signals != "" {
		body = json.RawMessage(signals)
	}
	resp := tb.tab.Do(t, http.MethodPost, path, body)
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	// An action without an sse parameter that succeeds answers with an empty body.
	if resp.Body == "" {
		return nil
	}
	return resp.Events(t)
}

type patch struct {
	Indexes   string           `json:"indexes"`
	Feed      map[string]*feed `json:"_feed"`
	Map       [][3]any         `json:"_map"`
	Portfolio *portfolio       `json:"_portfolio"`
	Pushed    int64            `json:"pushed"`
}

type feed struct {
	Bars   [][]float64 `json:"bars"`
	Price  string      `json:"price"`
	Change string      `json:"change"`
	Up     bool        `json:"up"`
}

func sortedKeys(m map[string]*feed) []string {
	if len(m) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(m))
}

func indexesParam(symbols []string) string {
	if len(symbols) == 0 {
		return "none"
	}
	return strings.Join(symbols, "-")
}

// requireSidebar checks that the sidebar lists every index and marks the selected ones.
func requireSidebar(t *testing.T, sidebar *html.Node, selected []string) {
	t.Helper()
	require.NotNil(t, sidebar, "the sidebar")
	require.Len(t, find(sidebar, class("index")), len(market.Indexes))
	require.Len(t, find(sidebar, attr("aria-pressed", "true")), len(selected))
	for _, sym := range selected {
		one(t, sidebar, "the selected button of "+sym, class("index"),
			attr("aria-pressed", "true"),
			attr("data-on:click", "@post('/toggle/"+sym+"/')"))
	}
}

// requireSelectButtons checks which of the buttons of
// the sidebar that show and hide all indexes are enabled.
func requireSelectButtons(t *testing.T, sidebar *html.Node, showAll, hideAll bool) {
	t.Helper()
	for title, enabled := range map[string]bool{
		"Show all indexes": showAll,
		"Hide all indexes": hideAll,
	} {
		_, disabled := datapagestest.Attr(
			one(t, sidebar, title, attr("title", title)), "disabled",
		)
		require.Equal(t, !enabled, disabled, "the button %q is disabled", title)
	}
}

// requireEmptyView checks that n shows the view of no indexes, whose button shows all.
func requireEmptyView(t *testing.T, n *html.Node) {
	t.Helper()
	one(t, n, "the empty view", class("empty"))
	one(t, n, "the button that shows all indexes", class("show-all"),
		attr("data-on:click", "@post('/select/all/')"))
}

func allIndexes() []string {
	symbols := make([]string, len(market.Indexes))
	for i, in := range market.Indexes {
		symbols[i] = in.Symbol
	}
	return symbols
}

// requireTabs checks that tabs links every view with query and
// marks the one of the current view.
func requireTabs(t *testing.T, tabs *html.Node, current string, q app.ViewQuery) {
	t.Helper()
	require.NotNil(t, tabs, "the tabs")
	cur := one(t, tabs, "the current tab", attr("aria-current", "page"))
	require.Len(t, find(tabs, tag("svg")), len(views), "every tab has an icon")
	require.Equal(t, "svg", children(cur)[0].Data, "the current tab starts with its icon")
	require.True(t, strings.HasSuffix(datapagestest.Text(cur), views[current].label),
		datapagestest.Text(cur))
	for name, v := range views {
		one(t, tabs, "the "+name+" tab", tag("a"), attr("href", v.href(q)))
	}
}

// selectOf returns the select bound to the signal name.
func selectOf(t *testing.T, doc *html.Node, name string) *html.Node {
	t.Helper()
	return one(t, doc, "the "+name+" select", tag("select"), attr("data-bind", name))
}

// requireSelected checks that value is the one selected option of sel.
func requireSelected(t *testing.T, sel *html.Node, value string) {
	t.Helper()
	selected := find(sel, tag("option"), func(n *html.Node) bool {
		_, ok := datapagestest.Attr(n, "selected")
		return ok
	})
	require.Len(t, selected, 1, "selected options of %s", attrOf(sel, "data-bind"))
	require.Equal(t, value, attrOf(selected[0], "value"))
}

func signalPatches(t *testing.T, evs []datapagestest.Event) []patch {
	t.Helper()
	var got []patch
	for _, ev := range evs {
		if ev.Type == datapagestest.PatchSignals {
			var p patch
			require.NoError(t, json.Unmarshal([]byte(ev.Data["signals"]), &p))
			got = append(got, p)
		}
	}
	return got
}

// pushesOf returns the signal patches among evs that are pushes of bars.
func pushesOf(t *testing.T, evs []datapagestest.Event) []patch {
	t.Helper()
	var pushes []patch
	for _, p := range signalPatches(t, evs) {
		if p.Pushed != 0 {
			pushes = append(pushes, p)
		}
	}
	return pushes
}

// onlyPatch returns the one signal patch among evs that is not a push.
func onlyPatch(t *testing.T, evs []datapagestest.Event) patch {
	t.Helper()
	var got []patch
	for _, p := range signalPatches(t, evs) {
		if p.Pushed == 0 {
			got = append(got, p)
		}
	}
	require.Len(t, got, 1)
	return got[0]
}

// patched returns the element with the given ID among the patches of evs,
// nil when none of them patches it.
func patched(evs []datapagestest.Event, id string) *html.Node {
	for _, ev := range evs {
		if ev.Type != datapagestest.PatchElements {
			continue
		}
		if n := datapagestest.ElementByID(ev.Root(), id); n != nil {
			return n
		}
	}
	return nil
}

// mustPatch returns the element patched returns and
// fails the test when no event patches it.
func mustPatch(t *testing.T, evs []datapagestest.Event, id string) *html.Node {
	t.Helper()
	n := patched(evs, id)
	require.NotNil(t, n, "no patch of the element with ID %q", id)
	return n
}

// match reports whether an element has a property.
type match func(*html.Node) bool

func tag(name string) match { return func(n *html.Node) bool { return n.Data == name } }

func attr(key, value string) match {
	return func(n *html.Node) bool {
		v, ok := datapagestest.Attr(n, key)
		return ok && v == value
	}
}

func attrContains(key, part string) match {
	return func(n *html.Node) bool {
		return strings.Contains(attrOf(n, key), part)
	}
}

func class(name string) match {
	return func(n *html.Node) bool {
		return hasClass(n, name)
	}
}

// find returns the elements under n, n included, that have every property.
func find(n *html.Node, matches ...match) []*html.Node {
	var found []*html.Node
	check := func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		for _, m := range matches {
			if !m(n) {
				return
			}
		}
		found = append(found, n)
	}
	check(n)
	for d := range n.Descendants() {
		check(d)
	}
	return found
}

// one returns the only element under n that has every property
// and fails the test unless there is exactly one.
func one(t *testing.T, n *html.Node, what string, matches ...match) *html.Node {
	t.Helper()
	found := find(n, matches...)
	require.Len(t, found, 1, what)
	return found[0]
}

func attrOf(n *html.Node, key string) string {
	v, _ := datapagestest.Attr(n, key)
	return v
}

func hasClass(n *html.Node, name string) bool {
	return slices.Contains(strings.Fields(attrOf(n, "class")), name)
}

// children returns the child elements of n.
func children(n *html.Node) []*html.Node {
	var kids []*html.Node
	for c := range n.ChildNodes() {
		if c.Type == html.ElementNode {
			kids = append(kids, c)
		}
	}
	return kids
}

func tagsOf(nodes []*html.Node) []string {
	tags := make([]string, len(nodes))
	for i, n := range nodes {
		tags[i] = n.Data
	}
	return tags
}
