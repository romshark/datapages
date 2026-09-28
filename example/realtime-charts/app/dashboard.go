package app

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/realtime-charts/market"
)

// chartType is the Lightweight Charts series type the charts of a view draw.
type chartType string

const (
	typeCandles   chartType = "candles"
	typeBars      chartType = "bars"
	typeLine      chartType = "line"
	typeArea      chartType = "area"
	typeBaseline  chartType = "baseline"
	typeHistogram chartType = "histogram"
)

var chartTypes = []chartType{
	typeCandles, typeBars, typeLine, typeArea, typeBaseline, typeHistogram,
}

// refreshRates are the intervals a tab can receive updates at,
// in the order the page offers them.
var refreshRates = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 30 * time.Second,
}

const (
	defaultRefresh = time.Second

	// defaultCharts is the number of indexes, from the top of the sidebar,
	// that a page without an indexes parameter shows.
	defaultCharts = 6

	// noIndexes is the indexes parameter of a page that shows none. An empty
	// parameter would be the one of a page that names none, which shows the
	// defaults.
	noIndexes = "none"
)

// Settings are the signals of every view page. View is the chart type of
// the page. Refresh, Indexes and Paused are bound to the navigation bar and
// the sidebar, and reflected into the query of the URL by [ViewQuery]. Pushed
// is the Unix second of the last push the tab received, which a stream that
// reconnects resumes from.
type Settings struct {
	View    string `json:"view"`
	Refresh string `json:"refresh"`
	Indexes string `json:"indexes"`
	Paused  int64  `json:"paused"`
	Pushed  int64  `json:"pushed"`
}

// ViewQuery are the query parameters of every view page. They seed the
// signals of the same names on page load, and Datapages writes every change
// of those signals back into the URL. A reload, a shared link and a tab to
// another view show the same settings.
//
// Indexes are the symbols of the shown indexes, separated by hyphens.
// Paused is the Unix second the charts are frozen at, zero on a live page.
// The URL of a live page carries no paused parameter: Datapages removes a
// reflected parameter whose signal is zero.
type ViewQuery struct {
	Refresh string `query:"refresh" reflectsignal:"refresh"`
	Indexes string `query:"indexes" reflectsignal:"indexes"`
	Paused  int64  `query:"paused" reflectsignal:"paused"`
}

// StateCharts is the view one tab shows and what its stream last sent it.
type StateCharts struct {
	Type    chartType
	Refresh time.Duration

	// Paused is the Unix second the charts of the tab are frozen at, zero
	// while they are live. The market keeps moving, and the first push after
	// the pause carries every bar that changed during it. Resumed tells
	// OnSettingsChanged to make that push without waiting for the next tick.
	Paused  int64
	Resumed bool

	// Indexes are the symbols of the indexes the tab asked for, Shown those
	// its page shows, both in the order of the sidebar. They differ from
	// POSTToggle until OnSettingsChanged.
	Indexes, Shown []string

	// Pushed is the Unix second of the last push to the tab, zero before the first.
	Pushed int64
}

// Dashboard holds the stream hooks and event handlers of every view page.
// The pages differ only in the chart type, which the view signal carries
// from the page to its stream.
type Dashboard struct{ App *App }

// StreamOpen rebuilds the state of the tab from its signals. The first
// stream of a page receives the last Window bars, those up to the second it
// is paused at on a paused page. A stream that reconnects, such as when a
// hidden tab is shown again, sends the second of the last push it received
// and gets the bars updated since, nothing while its tab is paused.
func (d Dashboard) StreamOpen(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateCharts],
	signals datapages.Signals[Settings],
) error {
	s := state.Values
	s.Type = chartType(signals.Values.View)
	if !slices.Contains(chartTypes, s.Type) {
		s.Type = typeCandles
	}
	s.Refresh = viewOf(ViewQuery{Refresh: signals.Values.Refresh})
	s.Indexes = parseIndexes(signals.Values.Indexes)
	s.Shown = s.Indexes
	s.Paused = d.App.pausedAt(signals.Values.Paused)
	// A second after the newest prices cannot come from this market.
	if p := signals.Values.Pushed; p > 0 && p <= d.App.now() {
		s.Pushed = p
	}
	switch {
	case s.Paused == 0:
		return d.push(sse, s)
	case s.Pushed == 0:
		// A paused page shows the charts of the second it is paused at.
		s.Pushed = s.Paused
		return sse.PatchSignals(signalPatch{
			Feed:   d.App.window(s.Type, s.Shown, s.Paused),
			Pushed: s.Paused,
		})
	}
	return nil
}

// OnIndexTicked runs for every index and every tick on every open tab. The
// first event of an index the tab shows pushes the charts of all indexes it
// shows, once the refresh interval of the tab has passed since the last push.
// The market writes the prices of every index before it publishes the first
// event.
func (d Dashboard) OnIndexTicked(
	event EventIndexTicked,
	sse datapages.SSE,
	state datapages.State[StateCharts],
) error {
	s := state.Values
	if s.Paused != 0 || !slices.Contains(s.Shown, string(event.Index)) ||
		time.Duration(d.App.now()-s.Pushed)*time.Second < s.Refresh {
		return nil
	}
	return d.push(sse, s)
}

// OnSettingsChanged renders the settings that POSTSettings and POSTToggle
// changed.
func (d Dashboard) OnSettingsChanged(
	_ EventSettingsChanged,
	sse datapages.SSE,
	state datapages.State[StateCharts],
) error {
	s := state.Values
	patch := signalPatch{Indexes: formatIndexes(s.Indexes)}
	var added []string
	for _, sym := range s.Indexes {
		if !slices.Contains(s.Shown, sym) {
			added = append(added, sym)
		}
	}
	if len(added) > 0 || len(s.Shown) != len(s.Indexes) {
		// The charts of a paused tab show the second it is paused at,
		// the ones it adds too.
		at := s.Paused
		if at == 0 {
			at = s.Pushed
		}
		patch.Feed = d.App.window(s.Type, added, at)
		// Deletes the signals of the removed charts.
		for _, sym := range s.Shown {
			if !slices.Contains(s.Indexes, sym) {
				patch.Feed[sym] = nil
			}
		}
	}

	// The bars go first. A chart that the grid adds finds its bars in place
	// when Datastar initializes its data-effect.
	if err := sse.PatchSignals(patch); err != nil {
		return err
	}
	if patch.Feed != nil {
		s.Shown = s.Indexes
		if err := sse.PatchElement(grid(s.Type, d.App.cards(s.Shown))); err != nil {
			return err
		}
		if err := sse.PatchElement(sidebar(s.Shown)); err != nil {
			return err
		}
	}
	// The links of the tabs carry the settings.
	q := queryOf(s.Refresh, s.Indexes, s.Paused)
	if err := sse.PatchElement(tabs(s.Type, q)); err != nil {
		return err
	}
	// Other settings changes wait for the next push, which keeps the refresh
	// interval of the tab.
	if s.Resumed {
		s.Resumed = false
		if d.App.now() > s.Pushed {
			if err := d.push(sse, s); err != nil {
				return err
			}
		}
	}
	return sse.PatchElement(errorMessage(""))
}

// push sends the tab the bars that changed since its last push, and the last
// Window bars when nothing was pushed yet.
func (d Dashboard) push(sse datapages.SSE, s *StateCharts) error {
	now := d.App.now()
	feed := d.App.window(s.Type, s.Shown, now)
	if s.Pushed != 0 {
		feed = d.App.since(s.Type, s.Shown, s.Pushed, now)
	}
	s.Pushed = now
	return sse.PatchSignals(signalPatch{Feed: feed, Pushed: now})
}

// page renders a view page whose charts draw t.
func (d Dashboard) page(t chartType, q ViewQuery) datapages.Component {
	refresh := viewOf(q)
	indexes := parseIndexes(q.Indexes)
	q = queryOf(refresh, indexes, d.App.pausedAt(q.Paused))
	return pageView(t, q, indexes, d.App.cards(indexes))
}

// parseRefresh reports the interval s names, if it is one of refreshRates.
func parseRefresh(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && slices.Contains(refreshRates, d)
}

// viewOf returns the refresh interval a page load or a new stream starts
// with. A value the page does not offer falls back to the default: a
// hand-edited URL shows the default view instead of an error.
func viewOf(q ViewQuery) time.Duration {
	refresh, ok := parseRefresh(q.Refresh)
	if !ok {
		return defaultRefresh
	}
	return refresh
}

// parseIndexes returns the known symbols of s in the order of the sidebar,
// and the first defaultCharts symbols when s is empty.
func parseIndexes(s string) []string {
	if s == "" {
		symbols := make([]string, defaultCharts)
		for i, in := range market.Indexes[:defaultCharts] {
			symbols[i] = in.Symbol
		}
		return symbols
	}
	var symbols []string
	for _, in := range market.Indexes {
		if slices.Contains(strings.Split(s, "-"), in.Symbol) {
			symbols = append(symbols, in.Symbol)
		}
	}
	return symbols
}

// formatIndexes returns the indexes parameter of symbols.
func formatIndexes(symbols []string) string {
	if len(symbols) == 0 {
		return noIndexes
	}
	return strings.Join(symbols, "-")
}

// toggled returns symbols with sym added or removed, in the order of the
// sidebar. It allocates a new slice: the state may share the old one with
// the shown indexes.
func toggled(symbols []string, sym string) []string {
	var out []string
	for _, in := range market.Indexes {
		if slices.Contains(symbols, in.Symbol) != (in.Symbol == sym) {
			out = append(out, in.Symbol)
		}
	}
	return out
}

// queryOf returns the query of a view page with the given settings.
func queryOf(refresh time.Duration, indexes []string, paused int64) ViewQuery {
	return ViewQuery{
		Refresh: refresh.String(), Indexes: formatIndexes(indexes), Paused: paused,
	}
}
