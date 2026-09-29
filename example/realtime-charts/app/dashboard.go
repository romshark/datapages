package app

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/realtime-charts/market"
)

type view string

const (
	viewCharts    view = "charts"
	viewMap       view = "map"
	viewPortfolio view = "portfolio"
)

var views = []view{viewCharts, viewMap, viewPortfolio}

// A chartType is the type attribute of <rt-chart>.
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

// The rows layout puts one chart across the page in each row, and the grid layout as many
// columns as fit.
type layout string

const (
	layoutRows layout = "rows"
	layoutGrid layout = "grid"
)

var layouts = []layout{layoutRows, layoutGrid}

// title returns the label of a chart type or a layout: Candles for candles.
func title(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

var refreshRates = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 30 * time.Second,
}

// The page offers the time ranges the history of the database spans, see [App.ranges].
var timeRanges = []time.Duration{
	5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour,
	3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// A time range takes the shortest bar length that keeps it within rangeBars bars.
var barLengths = []time.Duration{
	5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second,
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute,
	15 * time.Minute,
}

const (
	defaultType    = typeCandles
	defaultLayout  = layoutGrid
	defaultRefresh = time.Second
	defaultRange   = 15 * time.Minute

	// rangeBars is the most bars a chart shows over its time range. A card is 360 pixels
	// wide or more, which gives a bar 3 pixels or more.
	rangeBars = 120

	// defaultCharts is the number of indexes, from the top of the sidebar, that a page
	// without an indexes parameter shows.
	defaultCharts = 6

	// A page that shows no indexes has this indexes parameter: an empty one would show
	// the defaults.
	noIndexes = "none"
)

// Settings are the signals of every view page. View is what the page shows. Type,
// Refresh, Range, Indexes, Paused, Sync and Layout are bound to the navigation bar, the
// toolbar and the sidebar, and reflected into the query of the URL by [ViewQuery]. Pushed
// is the Unix second of the last push the tab received, which a stream that reconnects
// resumes from.
type Settings struct {
	View    string `json:"view"`
	Type    string `json:"type"`
	Refresh string `json:"refresh"`
	Range   string `json:"range"`
	Indexes string `json:"indexes"`
	Paused  int64  `json:"paused"`
	Sync    bool   `json:"sync"`
	Layout  string `json:"layout"`
	Pushed  int64  `json:"pushed"`
}

// ViewQuery seeds the signals of the same names on the load of a view page, and Datapages
// writes every change of those signals back into the URL. A reload, a shared link and a
// tab to another view show the same settings.
//
// The other pages keep Type, the chart type of the chart page, for its tab. Range names
// the time range, such as 15m or 24h. Indexes holds the symbols of the shown indexes,
// separated by hyphens. Paused is the Unix second the charts are frozen at, zero on a
// live page. Sync makes the charts follow the crosshair, the scrolling and the zoom of
// each other.
//
// The URL of a live page carries no paused parameter and the one of a page without sync
// no sync parameter: Datapages removes a reflected parameter whose signal is zero or
// false.
type ViewQuery struct {
	Type    string `query:"type" reflectsignal:"type"`
	Refresh string `query:"refresh" reflectsignal:"refresh"`
	Range   string `query:"range" reflectsignal:"range"`
	Indexes string `query:"indexes" reflectsignal:"indexes"`
	Paused  int64  `query:"paused" reflectsignal:"paused"`
	Sync    bool   `query:"sync" reflectsignal:"sync"`
	Layout  string `query:"layout" reflectsignal:"layout"`
}

// StateCharts is the view one tab shows and what its stream last sent it.
type StateCharts struct {
	View    view
	Refresh time.Duration

	// Type is what the tab asked for, ShownType what its charts show. They differ between
	// [App.POSTSettings] and [Dashboard.OnSettingsChanged]. The market map and the
	// portfolio page keep the type for the tab of the charts.
	Type, ShownType chartType

	// Paused is the Unix second the charts of the tab are frozen at, zero while they are
	// live. The market keeps moving, and the first push after the pause carries every bar
	// that changed during it. Resumed tells OnSettingsChanged to make that push without
	// waiting for the next tick.
	Paused  int64
	Resumed bool

	// Indexes are the symbols the tab asked for, Shown those its page shows, both in the
	// order of the sidebar. They differ between [App.POSTToggle] or [App.POSTSelect] and
	// [Dashboard.OnSettingsChanged].
	Indexes, Shown []string

	// Range is what the tab asked for, ShownRange what its charts show. They differ
	// between [App.POSTSettings] and [Dashboard.OnSettingsChanged].
	Range, ShownRange time.Duration

	// Sync is whether the charts of the tab follow each other, and Layout how the chart
	// page arranges them. The page reads both from their signals, and the tab keeps them
	// for the links of its tabs.
	Sync   bool
	Layout layout

	// Pushed is the Unix second of the last push to the tab, zero before the first.
	Pushed int64
}

// Dashboard holds the stream hooks and event handlers of every view page. The pages
// differ only in what they show, which the view signal carries from the page to its
// stream. [Dashboard.data] and [Dashboard.main] are where the market map and the
// portfolio differ from the charts.
type Dashboard struct{ App *App }

// StreamOpen rebuilds the state of the tab from its signals. The first stream of a page
// receives the bars of its whole time range, those up to the second it is paused at on a
// paused page. A stream that reconnects, such as when a hidden tab is shown again, sends
// the second of the last push it received and gets the bars updated since, nothing while
// its tab is paused.
func (d Dashboard) StreamOpen(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateCharts],
	signals datapages.Signals[Settings],
) error {
	s := state.Values
	s.View = view(signals.Values.View)
	if !slices.Contains(views, s.View) {
		s.View = viewCharts
	}
	s.Type = typeOf(signals.Values.Type)
	s.ShownType = s.Type
	s.Refresh = refreshOf(signals.Values.Refresh)
	s.Range = d.App.rangeOf(signals.Values.Range)
	s.ShownRange = s.Range
	s.Indexes = parseIndexes(signals.Values.Indexes)
	s.Shown = s.Indexes
	s.Paused = d.App.pausedAt(signals.Values.Paused)
	s.Sync = signals.Values.Sync
	s.Layout = layoutOf(signals.Values.Layout)
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
		patch := d.data(s, 0, s.Paused)
		patch.Pushed = s.Paused
		return sse.PatchSignals(patch)
	}
	return nil
}

// OnIndexTicked runs for every index and every tick on every open tab. The first event of
// an index the tab shows pushes the charts of all indexes it shows, once the refresh
// interval of the tab has passed since the last push. The market map shows every index.
// The market writes the prices of every index before it publishes the first event.
func (d Dashboard) OnIndexTicked(
	event EventIndexTicked,
	sse datapages.SSE,
	state datapages.State[StateCharts],
) error {
	s := state.Values
	shows := s.View == viewMap || slices.Contains(s.Shown, string(event.Index))
	if s.Paused != 0 || !shows ||
		time.Duration(d.App.now()-s.Pushed)*time.Second < s.Refresh {
		return nil
	}
	return d.push(sse, s)
}

// OnSettingsChanged renders the settings that [App.POSTSettings], [App.POSTToggle] and
// [App.POSTSelect] changed.
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
	changed := len(added) > 0 || len(s.Shown) != len(s.Indexes)
	// Another time range has bars of another length, and another chart type other points:
	// every chart starts over with the bars of the new ones.
	redrawn := s.View == viewCharts && (s.Range != s.ShownRange || s.Type != s.ShownType)
	if redrawn {
		added = s.Indexes
	}
	if changed || redrawn {
		// The charts of a paused tab show the second it is paused at, the ones it adds
		// too.
		at := s.Paused
		if at == 0 {
			at = s.Pushed
		}
		switch s.View {
		case viewMap:
			// The map shows every index. It outlines the selected ones, which it reads
			// from the indexes signal.
		case viewPortfolio:
			// Other holdings are another portfolio.
			patch.Portfolio = d.App.portfolio(s.Indexes, 0, at)
		default:
			patch.Feed = d.App.window(s.Type, added, s.Range, at)
			// A null feed deletes the signals of a removed chart.
			for _, sym := range s.Shown {
				if !slices.Contains(s.Indexes, sym) {
					patch.Feed[sym] = nil
				}
			}
		}
	}

	// The bars go first. A chart that the grid adds finds its bars in place when Datastar
	// initializes its data-effect.
	if err := sse.PatchSignals(patch); err != nil {
		return err
	}
	if changed || redrawn {
		s.Shown, s.ShownRange, s.ShownType = s.Indexes, s.Range, s.Type
		if err := sse.PatchElement(d.main(s.View, s.ShownType, s.Shown, s.ShownRange)); err != nil {
			return err
		}
	}
	if changed {
		if err := sse.PatchElement(sidebar(s.Shown)); err != nil {
			return err
		}
	}
	// The links of the tabs carry the settings.
	if err := sse.PatchElement(tabs(s.View, s.query())); err != nil {
		return err
	}
	// Other settings changes wait for the next push, which keeps the refresh interval of
	// the tab.
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

// push sends the tab the bars that changed since its last push, and the bars of its whole
// time range when nothing was pushed yet.
func (d Dashboard) push(sse datapages.SSE, s *StateCharts) error {
	now := d.App.now()
	patch := d.data(s, s.Pushed, now)
	s.Pushed = now
	patch.Pushed = now
	return sse.PatchSignals(patch)
}

// data returns the signals of the view of the tab with the bars up to the Unix second at:
// those that changed after the Unix second since, or the whole time range when since is
// zero.
func (d Dashboard) data(s *StateCharts, since, at int64) signalPatch {
	switch {
	case s.View == viewMap:
		// Every tile can change with every tick.
		return signalPatch{Map: d.App.marketMap(at)}
	case s.View == viewPortfolio:
		return signalPatch{Portfolio: d.App.portfolio(s.Shown, since, at)}
	case since == 0:
		return signalPatch{Feed: d.App.window(s.ShownType, s.Shown, s.ShownRange, at)}
	}
	return signalPatch{Feed: d.App.since(s.ShownType, s.Shown, s.ShownRange, since, at)}
}

// page renders the view page that shows v with the settings of q, those of them the page
// offers.
func (d Dashboard) page(v view, q ViewQuery) datapages.Component {
	s := StateCharts{
		Type:    typeOf(q.Type),
		Refresh: refreshOf(q.Refresh),
		Range:   d.App.rangeOf(q.Range),
		Indexes: parseIndexes(q.Indexes),
		Paused:  d.App.pausedAt(q.Paused),
		Sync:    q.Sync, Layout: layoutOf(q.Layout),
	}
	return pageView(
		v, s.query(), d.App.ranges(), s.Indexes, d.main(v, s.Type, s.Indexes, s.Range),
	)
}

// main returns the part of a view page that a selection, time range or chart type change
// patches as HTML: the grid of charts of type t, the market map or the panels of the
// portfolio.
func (d Dashboard) main(
	v view, t chartType, symbols []string, rng time.Duration,
) templ.Component {
	switch v {
	case viewMap:
		return marketMapView()
	case viewPortfolio:
		return portfolio(symbols, sectorShares(symbols))
	}
	return grid(t, d.App.cards(symbols), rng)
}

// parseType reports the chart type s names, if the page offers it.
func parseType(s string) (chartType, bool) {
	t := chartType(s)
	return t, slices.Contains(chartTypes, t)
}

// typeOf returns the chart type a page load or a new stream starts with, like
// [refreshOf].
func typeOf(s string) chartType {
	if t, ok := parseType(s); ok {
		return t
	}
	return defaultType
}

// parseLayout reports the layout s names, if the page offers it.
func parseLayout(s string) (layout, bool) {
	l := layout(s)
	return l, slices.Contains(layouts, l)
}

// layoutOf returns the layout a page load or a new stream starts with, like [refreshOf].
func layoutOf(s string) layout {
	if l, ok := parseLayout(s); ok {
		return l
	}
	return defaultLayout
}

// parseRefresh reports the interval s names, if it is one of refreshRates.
func parseRefresh(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && slices.Contains(refreshRates, d)
}

// refreshOf returns the refresh interval a page load or a new stream starts with. A value
// the page does not offer falls back to the default: a hand-edited URL shows the default
// view instead of an error.
func refreshOf(s string) time.Duration {
	if d, ok := parseRefresh(s); ok {
		return d
	}
	return defaultRefresh
}

// ranges returns the time ranges the page offers: those the history of the database
// spans, and the shortest one when it spans none.
func (a *App) ranges() []time.Duration {
	n := 1
	for n < len(timeRanges) && timeRanges[n] <= a.market.History() {
		n++
	}
	return timeRanges[:n]
}

// parseRange reports the time range s names, if the page offers it.
func (a *App) parseRange(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && slices.Contains(a.ranges(), d)
}

// rangeOf returns the time range a page load or a new stream starts with, like
// [refreshOf]: the default for a value the page does not offer, or the longest range
// offered when the history is shorter than the default.
func (a *App) rangeOf(s string) time.Duration {
	if d, ok := a.parseRange(s); ok {
		return d
	}
	return min(defaultRange, a.ranges()[len(a.ranges())-1])
}

func barOf(rng time.Duration) time.Duration {
	for _, bar := range barLengths {
		if rng/bar <= rangeBars {
			return bar
		}
	}
	return barLengths[len(barLengths)-1]
}

// rangeName returns 15m or 24h where the String of the duration would be 15m0s or
// 24h0m0s.
func rangeName(d time.Duration) string {
	if d%time.Hour == 0 {
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	}
	return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
}

// rangeLabel returns "Last 15 minutes" for 15 minutes and "Last 1 hour" for an hour.
func rangeLabel(d time.Duration) string {
	n, unit := int64(d/time.Minute), "minute"
	if d%time.Hour == 0 {
		n, unit = int64(d/time.Hour), "hour"
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("Last %d %s", n, unit)
}

// parseIndexes returns the known symbols of s in the order of the sidebar, and the first
// defaultCharts symbols when s is empty.
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

func formatIndexes(symbols []string) string {
	if len(symbols) == 0 {
		return noIndexes
	}
	return strings.Join(symbols, "-")
}

// toggled returns symbols with sym added or removed, in the order of the sidebar. It
// allocates a new slice: the state may share the old one with the shown indexes.
func toggled(symbols []string, sym string) []string {
	var out []string
	for _, in := range market.Indexes {
		if slices.Contains(symbols, in.Symbol) != (in.Symbol == sym) {
			out = append(out, in.Symbol)
		}
	}
	return out
}

func (s *StateCharts) query() ViewQuery {
	return ViewQuery{
		Type: string(s.Type), Refresh: s.Refresh.String(), Range: rangeName(s.Range),
		Indexes: formatIndexes(s.Indexes), Paused: s.Paused, Sync: s.Sync,
		Layout: string(s.Layout),
	}
}
