// Tests control market ticks so every HTTP stream push has a fixed time.

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

// t0 starts each test 2 seconds into a bar.
var t0 = time.Unix(1_800_000_002, 0)

const history = time.Hour

// defaults are shown when the URL omits indexes.
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

// TestPage tests the complete initial chart view.
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
		"no indexes": {
			typ:     "area",
			query:   "type=area&indexes=none",
			refresh: "1s",
		},
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
			at := cmp.Or(tc.paused, t0.Unix())
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
			}
			for _, sym := range tc.indexes {
				resp.ElementByID(t, "chart-"+sym+"-"+tc.typ+"-"+rng)
			}
			shown := chartsOf(t, doc)
			require.Equal(t, sortedOf(tc.indexes), sortedKeys(shown))
			for sym, c := range shown {
				e.requireWindowOf(t, sym, rng, at, c, shapeOf(tc.typ))
				require.Equal(t, tc.sync, c.Sync, sym)
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
				"layout": lay, "_pushed": float64(at),
			}, resp.Signals(t))
			refresh := selectOf(t, doc, "refresh")
			requireSelected(t, refresh, tc.refresh)
			requireSelected(t, selectOf(t, doc, "type"), tc.typ)
			rangeSelect := selectOf(t, doc, "range")
			requireSelected(t, rangeSelect, rng)
			require.Equal(t, "Time range", attrOf(rangeSelect, "title"))
			// A paused page can change range without changing its end time.
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
			// Visibility reconnects the stream without ending a pause.
			require.NotContains(t, resp.Body, "visibilitychange")
			requireTabs(t, resp.ElementByID(t, "tabs"), "charts", q)
		})
	}
}

// TestPageRejectsMalformedPause tests a non-numeric paused parameter.
func TestPageRejectsMalformedPause(t *testing.T) {
	resp := newEnv(t).get(t, views["charts"].href(app.ViewQuery{})+"?type=line&paused=soon")
	require.Equal(t, http.StatusBadRequest, resp.Status)
}

// TestStreamOpen tests the initial full-window stream render.
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
			r := e.openTab(t, "/", tc.signals).renders(t, 1)[0]
			require.Equal(t, t0.Unix(), r.At)
			charts := chartsOf(t, r.Main)
			require.Equal(t, tc.indexes, sortedKeys(charts))
			for sym, c := range charts {
				e.requireWindowOf(t, sym, cmp.Or(tc.rng, "15m"), t0.Unix(), c, tc.shape)
			}
		})
	}
}

// TestPushInterval tests refresh-rate throttling.
func TestPushInterval(t *testing.T) {
	for name, interval := range map[string]int{"1s": 1, "2s": 2, "5s": 5, "30s": 30} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tb := e.openTab(t, "/",
				`{"view":"charts","type":"candles","layout":"grid","refresh":"`+name+`","indexes":"AURA-GLWT-VRGE"}`)
			tb.renders(t, 1)

			for push := 1; push <= 2; push++ {
				// Wait before advancing the controlled market again.
				for s := (push-1)*interval + 1; s <= push*interval; s++ {
					e.tick(t, s)
				}
				r := tb.renders(t, push+1)[push]
				now := t0.Unix() + int64(push*interval)
				require.Equal(t, now, r.At, "push %d", push)
				charts := chartsOf(t, r.Main)
				require.Equal(t, []string{"AURA", "GLWT", "VRGE"}, sortedKeys(charts))
				for sym, c := range charts {
					e.requireWindow(t, sym, now, c, "ohlc")
				}
			}
			require.Len(t, tb.renders(t, 0), 3, "pushes between the intervals")
		})
	}
}

// TestHiddenIndexEvents tests that hidden indexes do not trigger a push.
func TestHiddenIndexEvents(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"DUNS"}`)
	tb.renders(t, 1)

	e.market.Tick(t0.Add(time.Second))
	e.publish(t, "AURA")
	e.publish(t, "VRGE")
	// The ordered settings event proves earlier hidden-index events were handled.
	tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`)
	renders := tb.renders(t, 2)
	require.Len(t, renders, 2, "pushes for hidden indexes")
	require.Equal(t, t0.Unix(), renders[1].At)

	e.publish(t, "DUNS")
	r := tb.renders(t, 3)[2]
	require.Equal(t, t0.Unix()+1, r.At)
	charts := chartsOf(t, r.Main)
	require.Equal(t, []string{"DUNS"}, sortedKeys(charts))
	e.requireWindow(t, "DUNS", t0.Unix()+1, charts["DUNS"], "ohlc")
}

// TestToggle tests one-index selection across the grid, sidebar, tabs and URL.
func TestToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.renders(t, 1)

	evs := tb.toggle(t, "CRUX")
	require.Equal(t, "AURA-BLTX-CRUX", indexesOf(t, evs))
	r := onlyRender(t, evs)
	require.Equal(t, t0.Unix(), r.At)
	charts := chartsOf(t, r.Main)
	require.Equal(t, []string{"AURA", "BLTX", "CRUX"}, sortedKeys(charts))
	for sym, c := range charts {
		e.requireWindow(t, sym, t0.Unix(), c, "close")
	}
	require.Len(t, find(r.Main, tag("rt-chart"), attr("type", "line")), 3)
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
	require.Equal(t, []string{"AURA", "BLTX", "CRUX"},
		sortedKeys(chartsOf(t, tb.renders(t, 3)[2].Main)))

	evs = tb.toggle(t, "AURA")
	require.Equal(t, "BLTX-CRUX", indexesOf(t, evs))
	require.Equal(t, []string{"BLTX", "CRUX"}, sortedKeys(chartsOf(t, onlyRender(t, evs).Main)))

	tb.toggle(t, "BLTX")
	evs = tb.toggle(t, "CRUX")
	require.Equal(t, "none", indexesOf(t, evs))
	requireEmptyView(t, onlyRender(t, evs).Main)

	resp := tb.post(t, "/toggle/XXXX/", "")
	require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
		"Something went wrong.")
}

// TestSelect tests all, none and disabled bulk-selection states.
func TestSelect(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"line","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.renders(t, 1)
	all := allIndexes()

	evs := tb.change(t, "/select/all/", "")
	require.Equal(t, strings.Join(all, "-"), indexesOf(t, evs))
	charts := chartsOf(t, onlyRender(t, evs).Main)
	require.Equal(t, sortedOf(all), sortedKeys(charts))
	for sym, c := range charts {
		e.requireWindow(t, sym, t0.Unix(), c, "close")
	}
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
	require.Equal(t, "none", indexesOf(t, evs))
	grid := onlyRender(t, evs).Main
	require.Empty(t, chartsOf(t, grid))
	one(t, grid, "the empty view", class("empty"))
	sidebar = mustPatch(t, evs, "sidebar")
	requireSidebar(t, sidebar, nil)
	requireSelectButtons(t, sidebar, true, false)

	resp := tb.post(t, "/select/some/", "")
	require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
		"Something went wrong.")
}

// TestTimeRange tests redraws, links and later pushes after a range change, including a
// paused tab.
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
			tb.renders(t, 1)

			evs := tb.settings(t, fmt.Sprintf(
				`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"%s","paused":%d}`, tc.rng, tc.paused,
			))
			at := cmp.Or(tc.paused, t0.Unix())
			r := onlyRender(t, evs)
			require.Equal(t, at, r.At)
			charts := chartsOf(t, r.Main)
			require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(charts))
			for sym, c := range charts {
				e.requireWindowOf(t, sym, tc.rng, at, c, "ohlc")
			}
			for _, sym := range []string{"AURA", "BLTX"} {
				require.NotNil(t,
					datapagestest.ElementByID(
						r.Main, "chart-"+sym+"-candles-"+tc.rng,
					), sym)
			}
			require.Len(t, find(r.Main, tag("rt-chart"),
				attr("bar", strconv.FormatInt(barOf[tc.rng]/1000, 10))), 2)
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
			r = tb.renders(t, 3)[2]
			require.Equal(t, t0.Unix()+1, r.At)
			charts = chartsOf(t, r.Main)
			require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(charts))
			for sym, c := range charts {
				e.requireWindowOf(t, sym, tc.rng, t0.Unix()+1, c, "ohlc")
			}
		})
	}
}

// TestTimeRangeWithoutCharts tests that non-chart views preserve
// a range without exposing chart controls or changing their content.
func TestTimeRangeWithoutCharts(t *testing.T) {
	for _, view := range []string{"map", "portfolio"} {
		t.Run(view, func(t *testing.T) {
			e := newEnv(t)
			path := views[view].href(app.ViewQuery{})
			doc := e.get(t, path).Document()
			require.Empty(t, find(doc, attr("data-bind", "type")))
			require.Empty(t, find(doc, attr("data-bind", "range")))

			tb := e.openTab(t, path, `{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
			first := tb.renders(t, 1)[0]
			evs := tb.settings(t, `{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"1h"}`)
			require.Equal(t, htmlOf(t, first.Main), htmlOf(t, onlyRender(t, evs).Main))
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

// TestChartType tests chart redraws and preservation across non-chart views.
func TestChartType(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.renders(t, 1)

	evs := tb.settings(t, `{"view":"charts","type":"histogram","layout":"grid","refresh":"1s","range":"15m"}`)
	r := onlyRender(t, evs)
	charts := chartsOf(t, r.Main)
	require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(charts))
	for sym, c := range charts {
		e.requireWindow(t, sym, t0.Unix(), c, "change")
	}
	require.Len(t, find(r.Main, tag("rt-chart"), attr("type", "histogram")), 2)
	for _, sym := range []string{"AURA", "BLTX"} {
		require.NotNil(t,
			datapagestest.ElementByID(r.Main, "chart-"+sym+"-histogram-15m"), sym)
	}
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts", app.ViewQuery{
		Type:    "histogram",
		Layout:  "grid",
		Refresh: "1s",
		Range:   "15m",
		Indexes: "AURA-BLTX",
	})

	e.tick(t, 1)
	for sym, c := range chartsOf(t, tb.renders(t, 3)[2].Main) {
		e.requireWindow(t, sym, t0.Unix()+1, c, "change")
	}

	for _, view := range []string{"map", "portfolio"} {
		other := e.openTab(t, views[view].href(app.ViewQuery{}),
			`{"view":"`+view+`","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
		first := other.renders(t, 1)[0]
		evs := other.settings(t, `{"view":"`+view+`","type":"bars","layout":"grid","refresh":"1s","range":"15m"}`)
		require.Equal(t, htmlOf(t, first.Main), htmlOf(t, onlyRender(t, evs).Main), view)
		requireTabs(t, mustPatch(t, evs, "tabs"), view,
			app.ViewQuery{Type: "bars", Layout: "grid", Refresh: "1s", Range: "15m", Indexes: "AURA"})
	}
}

// TestTimeRanges tests history-based options and fallback
// when history is shorter than the default.
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

			r := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","indexes":"AURA"}`).renders(t, 1)[0]
			e.requireWindowOf(t, "AURA", tc.dflt, t0.Unix(), chartsOf(t, r.Main)["AURA"], "ohlc")
		})
	}
}

// TestPause tests freezing and immediate catch-up on resume.
func TestPause(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-BLTX"}`)
	tb.renders(t, 1)

	paused := fmt.Sprintf(`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","paused":%d}`, t0.Unix())
	tb.settings(t, paused)
	for s := 1; s <= 5; s++ {
		e.tick(t, s)
	}
	// The ordered settings event proves all paused ticks were handled.
	tb.settings(t, paused)
	renders := tb.renders(t, 3)
	require.Len(t, renders, 3, "pushes during the pause")
	for _, r := range renders {
		require.Equal(t, t0.Unix(), r.At)
	}

	r := onlyRender(t, tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`))
	require.Equal(t, t0.Unix()+5, r.At)
	charts := chartsOf(t, r.Main)
	require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(charts))
	for sym, c := range charts {
		e.requireWindow(t, sym, t0.Unix()+5, c, "ohlc")
	}

	e.tick(t, 6)
	require.Equal(t, t0.Unix()+6, tb.renders(t, 5)[4].At)
}

// TestPausedPage tests a shared paused URL, including its unfinished bar.
func TestPausedPage(t *testing.T) {
	e := newEnv(t)
	signals := `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-GLWT"}`
	e.market.Tick(t0.Add(7 * time.Second))
	live := e.openTab(t, "/", signals).renders(t, 1)[0]
	require.Equal(t, t0.Unix()+7, live.At)

	// The bar forming at t0+7 closes, and the next one starts.
	e.market.Tick(t0.Add(20 * time.Second))
	pausedAt := fmt.Sprintf(`{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA-GLWT","paused":%d}`,
		t0.Unix()+7)
	tb := e.openTab(t, "/", pausedAt)
	paused := tb.renders(t, 1)[0]
	require.Equal(t, live.At, paused.At)
	require.Equal(t, htmlOf(t, live.Main), htmlOf(t, paused.Main))

	e.tick(t, 21)
	r := onlyRender(t, tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m"}`))
	for sym, c := range chartsOf(t, r.Main) {
		e.requireWindow(t, sym, t0.Unix()+21, c, "ohlc")
	}
}

// TestSync tests toggling and preserving sync across streams and views.
func TestSync(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	require.False(t, chartsOf(t, tb.renders(t, 1)[0].Main)["AURA"].Sync)

	evs := tb.settings(t, `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","sync":true}`)
	require.True(t, chartsOf(t, onlyRender(t, evs).Main)["AURA"].Sync)
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
	require.False(t, chartsOf(t, onlyRender(t, evs).Main)["AURA"].Sync)
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "1s",
			Range:   "15m",
			Indexes: "AURA",
		})

	synced := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA","sync":true}`)
	require.True(t, chartsOf(t, synced.renders(t, 1)[0].Main)["AURA"].Sync)
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
	}
}

// TestLayout tests layout changes and preservation across streams and views.
func TestLayout(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	first := tb.renders(t, 1)[0]

	evs := tb.settings(t, `{"view":"charts","type":"candles","layout":"rows","refresh":"1s","range":"15m"}`)
	require.Equal(t, htmlOf(t, first.Main), htmlOf(t, onlyRender(t, evs).Main))
	requireTabs(t, mustPatch(t, evs, "tabs"), "charts", app.ViewQuery{
		Type:    "candles",
		Layout:  "rows",
		Refresh: "1s",
		Range:   "15m",
		Indexes: "AURA",
	})

	rows := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"rows","refresh":"1s","range":"15m","indexes":"AURA"}`)
	rows.renders(t, 1)
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

// TestSettingsReachOnlyTheirTab tests state-addressed updates.
func TestSettingsReachOnlyTheirTab(t *testing.T) {
	e := newEnv(t)
	changing := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	other := e.openTab(t, "/", `{"view":"charts","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA"}`)
	changing.renders(t, 1)
	other.renders(t, 1)

	changing.toggle(t, "BLTX")

	// Any event for the other tab would arrive before this tick.
	e.tick(t, 1)
	r := other.renders(t, 2)[1]
	require.Nil(t, patched(other.st.Events(), "tabs"), "the other tab rendered a settings change")
	require.Equal(t, []string{"AURA"}, sortedKeys(chartsOf(t, r.Main)))
	require.Equal(t, []string{"AURA", "BLTX"},
		sortedKeys(chartsOf(t, changing.renders(t, 3)[2].Main)))
}

// TestSettingsRejectsUnknownValues tests validation without state changes.
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
			tb.renders(t, 1)

			resp := tb.post(t, "/settings/", signals)
			require.Contains(t, datapagestest.Text(mustPatch(t, resp, "error")),
				"The server rejected these settings.")

			e.tick(t, 1)
			r := tb.renders(t, 2)[1]
			require.Equal(t, t0.Unix()+1, r.At)
			e.requireWindow(t, "AURA", t0.Unix()+1, chartsOf(t, r.Main)["AURA"], "ohlc")
			require.Nil(t, patched(tb.st.Events(), "tabs"), "the stream rendered a settings change")
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

func newEnvWith(t *testing.T, history time.Duration) *env {
	t.Helper()
	db := tsdb.New(history)
	m := market.New(db, t0, history)
	// Tests publish faster than streams consume, so the buffer holds every test event.
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

// tick advances the controlled market and publishes its production events.
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

// Bar lengths in milliseconds keep each range within 120 bars.
var barOf = map[string]int64{
	"5m": 5_000, "15m": 10_000, "30m": 15_000, "1h": 30_000,
	"3h": 120_000, "6h": 300_000, "12h": 600_000, "24h": 900_000,
}

// requireWindow checks the default chart window and header at a second.
func (e *env) requireWindow(t *testing.T, sym string, at int64, c *chart, shape string) {
	t.Helper()
	e.requireWindowOf(t, sym, "15m", at, c, shape)
}

func (e *env) requireWindowOf(
	t *testing.T, sym, rng string, at int64, c *chart, shape string,
) {
	t.Helper()
	require.NotNil(t, c, sym)
	d, err := time.ParseDuration(rng)
	require.NoError(t, err)
	step := barOf[rng]
	to := at * 1000
	buckets := e.db.OHLC(sym, to-to%step-(d.Milliseconds()/step-1)*step, to, step)
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
	open := e.market.Open(indexOf(t, sym))
	require.Equal(t, fmt.Sprintf("%+.2f%%", (last.Value/open-1)*100), c.Change, sym)
	require.Equal(t, last.Value >= open, c.Up, sym)
}

func shapeOf(typ string) string {
	switch typ {
	case "candles", "bars":
		return "ohlc"
	case "histogram":
		return "change"
	}
	return "close"
}

type tab struct {
	env *env
	tab *datapagestest.Tab
	st  *datapagestest.Stream
}

// openTab loads a page and connects its stream with Datastar signals.
func (e *env) openTab(t *testing.T, path, signals string) *tab {
	t.Helper()
	page := e.v.Load(t, path)
	require.NotEmpty(t, page.Instance())
	return &tab{env: e, tab: page, st: page.Connect(t, json.RawMessage(signals))}
}

// renders waits for at least n complete renders.
func (tb *tab) renders(t *testing.T, n int) []render {
	t.Helper()
	var got []render
	tb.env.srv.WaitFor(t, fmt.Sprintf("%d renders", n), func() bool {
		got = rendersOf(t, tb.st.Events())
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

// change sends an action and returns its stream events.
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

// post sends an action with optional signals.
func (tb *tab) post(t *testing.T, path, signals string) []datapagestest.Event {
	t.Helper()
	var body any
	if signals != "" {
		body = json.RawMessage(signals)
	}
	resp := tb.tab.Do(t, http.MethodPost, path, body)
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	if resp.Body == "" {
		return nil
	}
	return resp.Events(t)
}

type chart struct {
	Bars   [][]float64
	Price  string
	Change string
	Up     bool
	Sync   bool
}

// chartsOf indexes rendered cards by symbol.
func chartsOf(t *testing.T, n *html.Node) map[string]*chart {
	t.Helper()
	charts := map[string]*chart{}
	for _, card := range find(n, tag("article"), class("card")) {
		sym := strings.TrimPrefix(attrOf(card, "id"), "card-")
		rt := one(t, card, "the chart of "+sym, tag("rt-chart"))
		c := &chart{Price: datapagestest.Text(one(t, card, "the price of "+sym, class("price")))}
		require.NoError(t, json.Unmarshal([]byte(attrOf(rt, "bars")), &c.Bars), sym)
		_, c.Sync = datapagestest.Attr(rt, "sync")
		change := one(t, card, "the change of "+sym, class("change"))
		c.Change = datapagestest.Text(change)
		c.Up = hasClass(change, "up")
		require.NotEqual(t, c.Up, hasClass(change, "down"), "the change of %s is up or down", sym)
		charts[sym] = c
	}
	return charts
}

func sortedKeys(m map[string]*chart) []string {
	if len(m) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(m))
}

func sortedOf(symbols []string) []string {
	if len(symbols) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(symbols))
}

func indexesParam(symbols []string) string {
	if len(symbols) == 0 {
		return "none"
	}
	return strings.Join(symbols, "-")
}

// requireSidebar checks order and selection.
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

// requireSelectButtons checks bulk-selection availability.
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

// requireEmptyView checks the no-index state.
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

// requireTabs checks view links, queries and selection.
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

func selectOf(t *testing.T, doc *html.Node, name string) *html.Node {
	t.Helper()
	return one(t, doc, "the "+name+" select", tag("select"), attr("data-bind", name))
}

// requireSelected checks the only selected option.
func requireSelected(t *testing.T, sel *html.Node, value string) {
	t.Helper()
	selected := find(sel, tag("option"), func(n *html.Node) bool {
		_, ok := datapagestest.Attr(n, "selected")
		return ok
	})
	require.Len(t, selected, 1, "selected options of %s", attrOf(sel, "data-bind"))
	require.Equal(t, value, attrOf(selected[0], "value"))
}

// render pairs a main-view patch with its displayed second.
type render struct {
	At   int64
	Main *html.Node
}

// rendersOf pairs each displayed-second patch with the preceding main patch.
func rendersOf(t *testing.T, evs []datapagestest.Event) []render {
	t.Helper()
	var got []render
	var main *html.Node
	for _, ev := range evs {
		switch ev.Type {
		case datapagestest.PatchElements:
			for _, id := range []string{"grid", "market", "portfolio"} {
				if n := datapagestest.ElementByID(ev.Root(), id); n != nil {
					main = n
				}
			}
		case datapagestest.PatchSignals:
			var s struct {
				Pushed int64 `json:"_pushed"`
			}
			require.NoError(t, json.Unmarshal([]byte(ev.Data["signals"]), &s))
			if s.Pushed != 0 {
				require.NotNil(t, main, "the main part of the page before its second")
				got = append(got, render{At: s.Pushed, Main: main})
				main = nil
			}
		}
	}
	return got
}

func onlyRender(t *testing.T, evs []datapagestest.Event) render {
	t.Helper()
	got := rendersOf(t, evs)
	require.Len(t, got, 1)
	return got[0]
}

func indexesOf(t *testing.T, evs []datapagestest.Event) string {
	t.Helper()
	var got []string
	for _, ev := range evs {
		if ev.Type != datapagestest.PatchSignals {
			continue
		}
		var s struct {
			Indexes *string `json:"indexes"`
		}
		require.NoError(t, json.Unmarshal([]byte(ev.Data["signals"]), &s))
		if s.Indexes != nil {
			got = append(got, *s.Indexes)
		}
	}
	require.Len(t, got, 1)
	return got[0]
}

func htmlOf(t *testing.T, n *html.Node) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, html.Render(&b, n))
	return b.String()
}

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

func mustPatch(t *testing.T, evs []datapagestest.Event, id string) *html.Node {
	t.Helper()
	n := patched(evs, id)
	require.NotNil(t, n, "no patch of the element with ID %q", id)
	return n
}

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
