// Tests the view pages, their streams and the actions over HTTP, sent the
// way Datastar sends them. The tests tick the market and publish its events
// themselves, which fixes the market time of every push.

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen"
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

// views are the view pages by chart type: the label of the tab and the
// link to the page with the given query.
var views = map[string]struct {
	label string
	href  func(q app.ViewQuery) string
}{
	"candles": {"Candles", func(q app.ViewQuery) string {
		return href.PageIndex(href.QueryPageIndex(q))
	}},
	"bars": {"Bars", func(q app.ViewQuery) string {
		return href.PageBars(href.QueryPageBars(q))
	}},
	"line": {"Line", func(q app.ViewQuery) string {
		return href.PageLine(href.QueryPageLine(q))
	}},
	"area": {"Area", func(q app.ViewQuery) string {
		return href.PageArea(href.QueryPageArea(q))
	}},
	"baseline": {"Baseline", func(q app.ViewQuery) string {
		return href.PageBaseline(href.QueryPageBaseline(q))
	}},
	"histogram": {"Histogram", func(q app.ViewQuery) string {
		return href.PageHistogram(href.QueryPageHistogram(q))
	}},
}

// TestPage tests the chart type, the charts, the sidebar, the tabs and the
// settings a page load shows.
func TestPage(t *testing.T) {
	e := newEnv(t)
	first := t0.Add(-history).Unix()
	for name, tc := range map[string]struct {
		view    string
		query   string
		indexes []string
		refresh string
		paused  int64
	}{
		"defaults": {view: "candles", indexes: defaults, refresh: "1s"},
		"from the query": {
			view: "bars", query: "refresh=5s&indexes=GLWT-DUNS",
			indexes: []string{"DUNS", "GLWT"}, refresh: "5s",
		},
		"unknown and repeated indexes": {
			view: "line", query: "indexes=GLWT-XXXX-GLWT",
			indexes: []string{"GLWT"}, refresh: "1s",
		},
		"no indexes": {view: "area", query: "indexes=none", refresh: "1s"},
		"unknown refresh interval": {
			view: "area", query: "refresh=7s", indexes: defaults, refresh: "1s",
		},
		"paused": {
			view: "baseline", query: fmt.Sprintf("paused=%d", t0.Unix()-60),
			indexes: defaults, refresh: "1s", paused: t0.Unix() - 60,
		},
		"paused before the history": {
			view: "histogram", query: fmt.Sprintf("paused=%d", first-600),
			indexes: defaults, refresh: "1s", paused: first,
		},
		"paused after the last tick": {
			view: "candles", query: fmt.Sprintf("paused=%d", t0.Unix()+600),
			indexes: defaults, refresh: "1s", paused: t0.Unix(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := views[tc.view].href(app.ViewQuery{})
			if tc.query != "" {
				path += "?" + tc.query
			}
			status, body := e.get(t, path)
			require.Equal(t, http.StatusOK, status)

			require.Equal(t, len(tc.indexes), strings.Count(body, "<rt-chart "))
			require.Equal(t, len(tc.indexes), strings.Count(body, ` type="`+tc.view+`"`))
			for _, sym := range tc.indexes {
				require.Contains(t, body, `id="chart-`+sym+`"`)
			}
			if len(tc.indexes) == 0 {
				require.Contains(t, body, `class="empty"`)
			}
			requireSidebar(t, element([]event{{
				name: "datastar-patch-elements",
				data: map[string]string{"elements": sidebarOf(body)},
			}}, "sidebar"), tc.indexes)

			q := app.ViewQuery{Refresh: tc.refresh, Indexes: indexesParam(tc.indexes), Paused: tc.paused}
			require.Contains(t, body, `data-signals:view="&#34;`+tc.view+`&#34;"`)
			require.Contains(t, body, `data-signals:indexes="&#34;`+q.Indexes+`&#34;"`)
			require.Contains(t, body, `data-signals:paused="`+strconv.FormatInt(tc.paused, 10)+`"`)
			require.Contains(t, body, `<option value="`+tc.refresh+`" selected>`)
			live := `class="live" title="Pause updates"`
			if tc.paused != 0 {
				live = `class="live paused" title="Resume updates"`
			}
			require.Contains(t, body, live)
			require.Equal(t, tc.paused != 0, strings.Contains(body, "<select data-bind=\"refresh\" disabled"))
			// A hidden tab shown again reconnects its stream instead of
			// reloading, which would end a pause.
			require.NotContains(t, body, "visibilitychange")
			requireTabs(t, tabsOf(body), tc.view, q)
		})
	}
}

// TestPageRejectsMalformedPause tests that a paused parameter that is not
// a number is a bad request.
func TestPageRejectsMalformedPause(t *testing.T) {
	status, _ := newEnv(t).get(t, "/line/?paused=soon")
	require.Equal(t, http.StatusBadRequest, status)
}

// TestStreamOpen tests that a new stream receives the last window of bars of
// the indexes its tab shows, as the points its chart type draws.
func TestStreamOpen(t *testing.T) {
	for name, tc := range map[string]struct {
		view    string
		signals string
		indexes []string
		shape   string
	}{
		"candles": {
			view: "candles", signals: `{"view":"candles","refresh":"2s","indexes":"GLWT-AURA"}`,
			indexes: []string{"AURA", "GLWT"}, shape: "ohlc",
		},
		"bars": {
			view: "bars", signals: `{"view":"bars","refresh":"2s","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "ohlc",
		},
		"line": {
			view: "line", signals: `{"view":"line","refresh":"2s","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"area": {
			view: "area", signals: `{"view":"area","refresh":"2s","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"baseline": {
			view: "baseline", signals: `{"view":"baseline","refresh":"2s","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "close",
		},
		"histogram": {
			view: "histogram", signals: `{"view":"histogram","refresh":"2s","indexes":"DUNS"}`,
			indexes: []string{"DUNS"}, shape: "change",
		},
		"unknown settings": {
			view: "candles", signals: `{"view":"pie","refresh":"7s"}`,
			indexes: defaults, shape: "ohlc",
		},
		"no indexes": {
			view: "line", signals: `{"view":"line","refresh":"1s","indexes":"none"}`,
			shape: "close",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			p := e.openTab(t, views[tc.view].href(app.ViewQuery{}), tc.signals).pushes(t, 1)[0]
			require.Equal(t, t0.Unix(), p.Pushed)
			require.Equal(t, tc.indexes, sortedKeys(p.Feed))
			for _, sym := range tc.indexes {
				e.requireWindow(t, sym, t0.Unix(), p.Feed[sym], tc.shape)
			}
		})
	}
}

// TestPushInterval tests that a stream receives the bars that changed since
// its previous push, once per refresh interval.
func TestPushInterval(t *testing.T) {
	for name, interval := range map[string]int{"1s": 1, "2s": 2, "5s": 5, "30s": 30} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tb := e.openTab(t, "/",
				`{"view":"candles","refresh":"`+name+`","indexes":"AURA-GLWT-VRGE"}`)
			tb.pushes(t, 1)

			for push := 1; push <= 2; push++ {
				// Waiting for each push keeps the market from moving on before
				// the stream handled the ticks of the interval.
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

// TestHiddenIndexEvents tests that the events of indexes a tab does not show
// push nothing to it.
func TestHiddenIndexEvents(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"candles","refresh":"1s","indexes":"DUNS"}`)
	tb.pushes(t, 1)

	e.market.Tick(t0.Add(time.Second))
	e.publish(t, "AURA")
	e.publish(t, "VRGE")
	// The broker delivers the events of a stream in the order they were
	// published, and a settings change pushes nothing: once it is rendered,
	// the stream handled the events of the hidden indexes.
	tb.settings(t, `{"view":"candles","refresh":"1s"}`)
	require.Len(t, tb.pushes(t, 1), 1, "pushes for hidden indexes")

	e.publish(t, "DUNS")
	p := tb.pushes(t, 2)[1]
	require.Equal(t, t0.Unix()+1, p.Pushed)
	require.Equal(t, []string{"DUNS"}, sortedKeys(p.Feed))
	e.requireSince(t, "DUNS", t0.Unix(), t0.Unix()+1, p.Feed["DUNS"], "ohlc")
}

// TestToggle tests that a toggle shows or hides the chart of an index: the
// charts it adds get their window of bars, the charts it removes lose their
// signals, and the sidebar, the grid, the tabs and the URL follow.
func TestToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/line/", `{"view":"line","refresh":"2s","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)

	evs := tb.toggle(t, "CRUX")
	p := onlyPatch(t, evs)
	require.Equal(t, "AURA-BLTX-CRUX", p.Indexes)
	require.Equal(t, []string{"CRUX"}, sortedKeys(p.Feed))
	e.requireWindow(t, "CRUX", t0.Unix(), p.Feed["CRUX"], "close")
	grid := element(evs, "grid")
	require.Equal(t, 3, strings.Count(grid, "<rt-chart "))
	require.Equal(t, 3, strings.Count(grid, ` type="line"`))
	requireSidebar(t, element(evs, "sidebar"), []string{"AURA", "BLTX", "CRUX"})
	requireTabs(t, element(evs, "tabs"), "line",
		app.ViewQuery{Refresh: "2s", Indexes: "AURA-BLTX-CRUX"})

	e.tick(t, 1)
	e.tick(t, 2)
	require.Equal(t, []string{"AURA", "BLTX", "CRUX"}, sortedKeys(tb.pushes(t, 2)[1].Feed))

	evs = tb.toggle(t, "AURA")
	p = onlyPatch(t, evs)
	require.Equal(t, "BLTX-CRUX", p.Indexes)
	require.Equal(t, map[string]*feed{"AURA": nil}, p.Feed)
	require.Equal(t, 2, strings.Count(element(evs, "grid"), "<rt-chart "))

	tb.toggle(t, "BLTX")
	evs = tb.toggle(t, "CRUX")
	require.Equal(t, "none", onlyPatch(t, evs).Indexes)
	require.Contains(t, element(evs, "grid"), `class="empty"`)

	resp := tb.post(t, "/toggle/XXXX/", "")
	require.Contains(t, element(resp, "error"), "Something went wrong.")
}

// TestPause tests that a paused tab receives no push while the market moves,
// and every bar that changed during the pause once it resumes.
func TestPause(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"candles","refresh":"1s","indexes":"AURA-BLTX"}`)
	tb.pushes(t, 1)

	paused := fmt.Sprintf(`{"view":"candles","refresh":"1s","paused":%d}`, t0.Unix())
	tb.settings(t, paused)
	for s := 1; s <= 5; s++ {
		e.tick(t, s)
	}
	// The settings change after the ticks arrives after them: once it is
	// rendered, the stream handled every tick during the pause.
	tb.settings(t, paused)
	require.Len(t, tb.pushes(t, 1), 1, "pushes during the pause")

	pushes := pushesOf(t, tb.settings(t, `{"view":"candles","refresh":"1s"}`))
	require.Len(t, pushes, 1)
	require.Equal(t, t0.Unix()+5, pushes[0].Pushed)
	for sym, c := range pushes[0].Feed {
		e.requireSince(t, sym, t0.Unix(), t0.Unix()+5, c, "ohlc")
	}

	e.tick(t, 6)
	require.Equal(t, t0.Unix()+6, tb.pushes(t, 3)[2].Pushed)
}

// TestPauseLongerThanTheWindow tests that a tab resuming after a pause
// longer than the first window receives every bar of the pause.
func TestPauseLongerThanTheWindow(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/line/", `{"view":"line","refresh":"1s","indexes":"AURA"}`)
	tb.pushes(t, 1)
	tb.settings(t, fmt.Sprintf(`{"view":"line","refresh":"1s","paused":%d}`, t0.Unix()))

	// No events: the tab would ignore them.
	e.market.Tick(t0.Add(30 * time.Minute))
	pushes := pushesOf(t, tb.settings(t, `{"view":"line","refresh":"1s"}`))
	require.Len(t, pushes, 1)
	now := t0.Add(30 * time.Minute).Unix()
	require.Greater(t, len(pushes[0].Feed["AURA"].Bars), 120)
	e.requireSince(t, "AURA", t0.Unix(), now, pushes[0].Feed["AURA"], "close")
}

// TestPausedPage tests that a page opened paused at a second shows what a
// live page showed at that second, the bar that was forming included.
func TestPausedPage(t *testing.T) {
	e := newEnv(t)
	signals := `{"view":"candles","refresh":"1s","indexes":"AURA-GLWT"}`
	e.market.Tick(t0.Add(7 * time.Second))
	live := e.openTab(t, "/", signals).pushes(t, 1)[0]
	require.Equal(t, t0.Unix()+7, live.Pushed)

	// The bar forming at t0+7 closes, and the next one starts.
	e.market.Tick(t0.Add(20 * time.Second))
	pausedAt := fmt.Sprintf(`{"view":"candles","refresh":"1s","indexes":"AURA-GLWT","paused":%d}`,
		t0.Unix()+7)
	tb := e.openTab(t, "/", pausedAt)
	p := tb.pushes(t, 1)[0]
	require.Equal(t, live, p)

	e.tick(t, 21)
	pushes := pushesOf(t, tb.settings(t, `{"view":"candles","refresh":"1s"}`))
	require.Len(t, pushes, 1)
	for sym, c := range pushes[0].Feed {
		e.requireSince(t, sym, t0.Unix()+7, t0.Unix()+21, c, "ohlc")
	}
}

// TestStreamOpenResumes tests that a stream reconnecting with the second of
// the last push its tab received gets the bars updated since, and nothing
// until its tab resumes when it is paused.
func TestStreamOpenResumes(t *testing.T) {
	for name, tc := range map[string]struct {
		pushed int64
		paused bool
		since  int64 // the second the first push continues from, zero for the window
	}{
		"live":                    {pushed: t0.Unix() - 12, since: t0.Unix() - 12},
		"from before the history": {pushed: t0.Unix() - 7200, since: t0.Unix() - 7200},
		"after the last tick":     {pushed: t0.Unix() + 60},
		"paused":                  {pushed: t0.Unix() - 12, paused: true, since: t0.Unix() - 12},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			paused := int64(0)
			if tc.paused {
				paused = tc.pushed
			}
			tb := e.openTab(t, "/", fmt.Sprintf(
				`{"view":"candles","refresh":"1s","indexes":"AURA","paused":%d,"pushed":%d}`,
				paused, tc.pushed,
			))
			if tc.paused {
				tb.settings(t, `{"view":"candles","refresh":"1s"}`)
			}
			// A paused tab gets only the push of its resume: StreamOpen
			// pushed nothing.
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

// TestSettingsReachOnlyTheirTab tests that a settings change leaves another
// tab of the same view alone.
func TestSettingsReachOnlyTheirTab(t *testing.T) {
	e := newEnv(t)
	changing := e.openTab(t, "/", `{"view":"candles","refresh":"1s","indexes":"AURA"}`)
	other := e.openTab(t, "/", `{"view":"candles","refresh":"1s","indexes":"AURA"}`)
	changing.pushes(t, 1)
	other.pushes(t, 1)

	changing.toggle(t, "BLTX")

	// Had the other tab received the settings event, it would have arrived
	// before the tick.
	e.tick(t, 1)
	other.pushes(t, 2)
	for _, ev := range other.events() {
		require.Equal(t, "datastar-patch-signals", ev.name)
	}
	require.Equal(t, []string{"AURA"}, sortedKeys(other.pushes(t, 2)[1].Feed))
	require.Equal(t, []string{"AURA", "BLTX"}, sortedKeys(changing.pushes(t, 2)[1].Feed))
}

// TestSettingsRejectsUnknownInterval tests that the settings action rejects
// a refresh interval the page does not offer and keeps the tab's settings.
func TestSettingsRejectsUnknownInterval(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, "/", `{"view":"candles","refresh":"1s","indexes":"AURA"}`)
	tb.pushes(t, 1)

	resp := tb.post(t, "/settings/", `{"view":"candles","refresh":"3s"}`)
	require.Contains(t, element(resp, "error"), "The server rejected these settings.")

	e.tick(t, 1)
	p := tb.pushes(t, 2)[1]
	require.Equal(t, t0.Unix()+1, p.Pushed)
	for _, ev := range tb.events() {
		require.Equal(t, "datastar-patch-signals", ev.name)
	}
}

// env is a server, the market that writes its prices and the database
// that holds them.
type env struct {
	srv    *httptest.Server
	market *market.Market
	db     *tsdb.DB
	broker messaging.Broker
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := tsdb.New(history)
	m := market.New(db, t0, history)
	// The tests publish ticks faster than a stream handles them.
	// A buffer larger than the events of any test keeps the broker
	// from dropping one.
	broker := inmem.New(4096)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.New(db, m), broker, datapages.WithAssets(app.StaticFS, false))
	require.NoError(t, err)
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return &env{srv: srv, market: m, db: db, broker: broker}
}

func (e *env) get(t *testing.T, path string) (status int, body string) {
	t.Helper()
	resp, err := e.srv.Client().Get(e.srv.URL + path)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(b)
}

// tick moves the market to the given number of seconds after t0 and
// publishes the events the market loop of cmd/server publishes.
func (e *env) tick(t *testing.T, seconds int) {
	t.Helper()
	e.market.Tick(t0.Add(time.Duration(seconds) * time.Second))
	for _, in := range market.Indexes {
		e.publish(t, in.Symbol)
	}
}

// publish publishes the event of the index with the given symbol.
func (e *env) publish(t *testing.T, symbol string) {
	t.Helper()
	ev, err := json.Marshal(app.EventIndexTicked{Index: datapages.Subject(symbol)})
	require.NoError(t, err)
	require.NoError(t, e.broker.Publish(context.Background(), messaging.NoopMetrics{},
		datapagesgen.EvPrefixIndexTicked+symbol, ev))
}

// requireWindow checks that c carries the last window of bars of sym up to
// the second at, as points of the given shape, and the price at that second.
func (e *env) requireWindow(t *testing.T, sym string, at int64, c *feed, shape string) {
	t.Helper()
	to := at * 1000
	e.requireChart(t, sym, to-to%5000-119*5000, to, c, shape)
}

// requireSince checks that c carries the bars of sym that changed after the
// second pushed, up to the second now, and the price at now.
func (e *env) requireSince(t *testing.T, sym string, pushed, now int64, c *feed, shape string) {
	t.Helper()
	from := pushed * 1000
	e.requireChart(t, sym, from-from%5000, now*1000, c, shape)
}

func (e *env) requireChart(t *testing.T, sym string, from, to int64, c *feed, shape string) {
	t.Helper()
	require.NotNil(t, c, sym)
	buckets := e.db.OHLC(sym, from, to, 5000)
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

// tab is a browser tab: the instance ID its page load minted
// and what its stream delivered.
type tab struct {
	env      *env
	instance string

	mu     sync.Mutex
	stream strings.Builder
}

// openTab loads the page at path and connects its stream, which sends the
// signals in its query as Datastar does.
func (e *env) openTab(t *testing.T, path, signals string) *tab {
	t.Helper()
	page, err := e.srv.Client().Get(e.srv.URL + path)
	require.NoError(t, err)
	_ = page.Body.Close()
	require.Equal(t, http.StatusOK, page.StatusCode)
	tb := &tab{env: e, instance: page.Header.Get("Datapages-Instance")}
	require.NotEmpty(t, tb.instance)

	// Registered after srv.Close, hence run before it: Close waits for the
	// stream to end.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.srv.URL+path+"_$/?datastar="+url.QueryEscape(signals), nil)
	require.NoError(t, err)
	tb.header(req)
	resp, err := e.srv.Client().Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	go func() {
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 32<<10)
		for {
			n, err := resp.Body.Read(buf)
			tb.mu.Lock()
			tb.stream.Write(buf[:n])
			tb.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return tb
}

func (tb *tab) header(req *http.Request) {
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instance)
	// An uncompressed stream can be read event by event as it arrives.
	req.Header.Set("Accept-Encoding", "identity")
}

// events returns the complete events the stream delivered so far.
func (tb *tab) events() []event {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return events(tb.stream.String())
}

// waitFor waits until the events the stream delivered after the first skip
// satisfy done, and returns them.
func (tb *tab) waitFor(t *testing.T, skip int, what string, done func([]event) bool) []event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		evs := tb.events()[skip:]
		if done(evs) {
			return evs
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "the stream never delivered "+what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// pushes waits until the stream delivered at least n pushes of bars
// and returns all of them.
func (tb *tab) pushes(t *testing.T, n int) []patch {
	t.Helper()
	var got []patch
	tb.waitFor(t, 0, fmt.Sprintf("%d pushes", n), func(evs []event) bool {
		got = pushesOf(t, evs)
		return len(got) >= n
	})
	return got
}

// settings sends the settings action with the given signals and returns
// the events the stream delivered for it.
func (tb *tab) settings(t *testing.T, signals string) []event {
	t.Helper()
	return tb.change(t, "/settings/", signals)
}

// toggle sends the toggle action for the index with the given symbol and
// returns the events the stream delivered for it.
func (tb *tab) toggle(t *testing.T, symbol string) []event {
	t.Helper()
	return tb.change(t, "/toggle/"+symbol+"/", "")
}

// change sends an action that dispatches a settings change and returns the
// events the stream delivered for it. The action itself answers nothing.
func (tb *tab) change(t *testing.T, path, signals string) []event {
	t.Helper()
	skip := len(tb.events())
	require.Empty(t, tb.post(t, path, signals))
	// OnSettingsChanged patches the error message last.
	return tb.waitFor(t, skip, "the settings change", func(evs []event) bool {
		return element(evs, "error") != ""
	})
}

// post sends an action and returns the events of its response.
func (tb *tab) post(t *testing.T, path, signals string) []event {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		tb.env.srv.URL+path, strings.NewReader(signals))
	require.NoError(t, err)
	tb.header(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := tb.env.srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return events(string(b))
}

// patch is a signal patch as the page receives it.
type patch struct {
	Indexes string           `json:"indexes"`
	Feed    map[string]*feed `json:"_feed"`
	Pushed  int64            `json:"pushed"`
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

// indexesParam returns the indexes parameter of symbols.
func indexesParam(symbols []string) string {
	if len(symbols) == 0 {
		return "none"
	}
	return strings.Join(symbols, "-")
}

// requireSidebar checks that the sidebar lists every index and marks the
// selected ones.
func requireSidebar(t *testing.T, sidebar string, selected []string) {
	t.Helper()
	require.NotEmpty(t, sidebar)
	require.Equal(t, len(market.Indexes), strings.Count(sidebar, `class="index"`))
	require.Equal(t, len(selected), strings.Count(sidebar, `aria-pressed="true"`))
	for _, sym := range selected {
		require.Contains(t, sidebar,
			`aria-pressed="true" data-on:click="@post(&#39;/toggle/`+sym+`/&#39;)"`)
	}
}

// requireTabs checks that tabs links every view with query and marks the
// one of the current view.
func requireTabs(t *testing.T, tabs, current string, q app.ViewQuery) {
	t.Helper()
	require.NotEmpty(t, tabs)
	require.Equal(t, 1, strings.Count(tabs, `aria-current="page"`))
	require.Contains(t, tabs, `aria-current="page">`+views[current].label+`</a>`)
	for name, v := range views {
		require.Contains(t, tabs, `href="`+html.EscapeString(v.href(q))+`"`, name)
	}
}

// tabsOf and sidebarOf return the tabs and the sidebar of a page.
func tabsOf(page string) string {
	_, rest, _ := strings.Cut(page, `<nav id="tabs"`)
	tabs, _, _ := strings.Cut(rest, "</nav>")
	return `<nav id="tabs"` + tabs
}

func sidebarOf(page string) string {
	_, rest, _ := strings.Cut(page, `<aside id="sidebar"`)
	sidebar, _, _ := strings.Cut(rest, "</aside>")
	return `<aside id="sidebar"` + sidebar
}

// event is a server-sent event. Datastar starts each data line with the
// name of a field, such as "signals", "elements" or "mode".
type event struct {
	name string
	data map[string]string
}

// events parses the complete events in s. A field on several data lines
// is joined with newlines.
func events(s string) []event {
	var evs []event
	for {
		end := strings.Index(s, "\n\n")
		if end < 0 {
			return evs
		}
		ev := event{data: map[string]string{}}
		for line := range strings.SplitSeq(s[:end], "\n") {
			if name, ok := strings.CutPrefix(line, "event: "); ok {
				ev.name = name
			} else if data, ok := strings.CutPrefix(line, "data: "); ok {
				field, value, _ := strings.Cut(data, " ")
				if prev, ok := ev.data[field]; ok {
					value = prev + "\n" + value
				}
				ev.data[field] = value
			}
		}
		evs = append(evs, ev)
		s = s[end+2:]
	}
}

func signalPatches(t *testing.T, evs []event) []patch {
	t.Helper()
	var got []patch
	for _, ev := range evs {
		if ev.name == "datastar-patch-signals" {
			var p patch
			require.NoError(t, json.Unmarshal([]byte(ev.data["signals"]), &p))
			got = append(got, p)
		}
	}
	return got
}

// pushesOf returns the signal patches among evs that are pushes of bars.
func pushesOf(t *testing.T, evs []event) []patch {
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
func onlyPatch(t *testing.T, evs []event) patch {
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

// element returns the patched element with the given ID, or an empty string.
func element(evs []event, id string) string {
	for _, ev := range evs {
		if ev.name != "datastar-patch-elements" {
			continue
		}
		html := ev.data["elements"]
		if tag, _, ok := strings.Cut(html, ">"); ok && strings.Contains(tag, ` id="`+id+`"`) {
			return html
		}
	}
	return ""
}
