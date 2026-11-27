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

type layout string

const (
	layoutRows layout = "rows"
	layoutGrid layout = "grid"
)

var layouts = []layout{layoutRows, layoutGrid}

func title(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

var refreshRates = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 30 * time.Second,
}

// timeRanges lists selectable ranges.
// [App.ranges] caps them to retained history.
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

	// rangeBars keeps each bar at least 3 pixels wide in a 360-pixel card.
	rangeBars = 120

	// defaultCharts is the number of indexes shown when the URL omits them.
	defaultCharts = 6

	// noIndexes distinguishes no selection from the omitted default.
	noIndexes = "none"
)

// Settings are the shared view signals reflected by [ViewQuery].
type Settings struct {
	View    string `json:"view"`
	Type    string `json:"type"`
	Refresh string `json:"refresh"`
	Range   string `json:"range"`
	Indexes string `json:"indexes"`
	Paused  int64  `json:"paused"`
	Sync    bool   `json:"sync"`
	Layout  string `json:"layout"`
}

// ViewQuery reflects shared settings in the URL. Datapages omits zero Paused and Sync.
type ViewQuery struct {
	Type    string `query:"type" reflectsignal:"type"`
	Refresh string `query:"refresh" reflectsignal:"refresh"`
	Range   string `query:"range" reflectsignal:"range"`
	Indexes string `query:"indexes" reflectsignal:"indexes"`
	Paused  int64  `query:"paused" reflectsignal:"paused"`
	Sync    bool   `query:"sync" reflectsignal:"sync"`
	Layout  string `query:"layout" reflectsignal:"layout"`
}

// StateCharts holds the server-side view state of 1 tab.
type StateCharts struct {
	View    view
	Refresh time.Duration

	// Type persists across views for the chart page.
	Type chartType

	// Paused is 0 while live. Resumed requests an immediate catch-up push.
	Paused  int64
	Resumed bool

	Indexes []string
	Range   time.Duration

	Sync   bool
	Layout layout

	// Pushed is the displayed Unix second, or 0 before the first render.
	Pushed int64
}

// Dashboard holds the stream hooks and event handlers of every view page.
//
// Each push morphs a complete view. Stream compression removes repeated bytes.
type Dashboard struct{ App *App }

// StreamOpen restores tab state from signals and renders a complete view.
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
	s.Refresh = refreshOf(signals.Values.Refresh)
	s.Range = d.App.rangeOf(signals.Values.Range)
	s.Indexes = parseIndexes(signals.Values.Indexes)
	s.Paused = d.App.pausedAt(signals.Values.Paused)
	s.Sync = signals.Values.Sync
	s.Layout = layoutOf(signals.Values.Layout)
	return d.render(sse, s, d.App.shownAt(s.Paused))
}

// OnIndexTicked pushes the first shown index after the refresh interval. The market map
// treats every index as shown. The market writes all prices before publishing events.
func (d Dashboard) OnIndexTicked(
	event EventIndexTicked,
	sse datapages.SSE,
	state datapages.State[StateCharts],
) error {
	s := state.Values
	now := d.App.now()
	shows := s.View == viewMap || slices.Contains(s.Indexes, string(event.Index))
	if s.Paused != 0 || !shows || time.Duration(now-s.Pushed)*time.Second < s.Refresh {
		return nil
	}
	return d.render(sse, s, now)
}

// OnSettingsChanged renders the settings that [App.POSTSettings],
// [App.POSTToggle] and [App.POSTSelect] changed.
func (d Dashboard) OnSettingsChanged(
	_ EventSettingsChanged,
	sse datapages.SSE,
	state datapages.State[StateCharts],
) error {
	s := state.Values
	if err := sse.PatchSignals(indexesSignal{formatIndexes(s.Indexes)}); err != nil {
		return err
	}
	// Keep the prior second until the next tick unless this resumes a pause.
	at := s.Pushed
	switch {
	case s.Paused != 0:
		at = s.Paused
	case s.Resumed:
		at = d.App.now()
	}
	s.Resumed = false
	if err := d.render(sse, s, at); err != nil {
		return err
	}
	if err := sse.PatchElement(sidebar(s.Indexes)); err != nil {
		return err
	}
	if err := sse.PatchElement(tabs(s.View, s.query())); err != nil {
		return err
	}
	return sse.PatchElement(errorMessage(""))
}

func (d Dashboard) render(sse datapages.SSE, s *StateCharts, at int64) error {
	s.Pushed = at
	if err := sse.PatchElement(d.main(s, at)); err != nil {
		return err
	}
	return sse.PatchSignals(pushedSignal{at})
}

func (d Dashboard) page(v view, q ViewQuery) datapages.Component {
	s := StateCharts{
		View:    v,
		Type:    typeOf(q.Type),
		Refresh: refreshOf(q.Refresh),
		Range:   d.App.rangeOf(q.Range),
		Indexes: parseIndexes(q.Indexes),
		Paused:  d.App.pausedAt(q.Paused),
		Sync:    q.Sync, Layout: layoutOf(q.Layout),
	}
	at := d.App.shownAt(s.Paused)
	return pageView(v, s.query(), d.App.ranges(), s.Indexes, at, d.main(&s, at))
}

func (d Dashboard) main(s *StateCharts, at int64) templ.Component {
	switch s.View {
	case viewMap:
		return marketMapView(d.App.marketMap(at), formatIndexes(s.Indexes))
	case viewPortfolio:
		return portfolio(
			s.Indexes, sectorShares(s.Indexes), d.App.portfolio(s.Indexes, at),
		)
	}
	return grid(s.Type, d.App.cards(s.Type, s.Indexes, s.Range, at), s.Range, s.Sync)
}

func parseType(s string) (chartType, bool) {
	t := chartType(s)
	return t, slices.Contains(chartTypes, t)
}

func typeOf(s string) chartType {
	if t, ok := parseType(s); ok {
		return t
	}
	return defaultType
}

func parseLayout(s string) (layout, bool) {
	l := layout(s)
	return l, slices.Contains(layouts, l)
}

func layoutOf(s string) layout {
	if l, ok := parseLayout(s); ok {
		return l
	}
	return defaultLayout
}

func parseRefresh(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && slices.Contains(refreshRates, d)
}

// refreshOf uses the default for unsupported URL values.
func refreshOf(s string) time.Duration {
	if d, ok := parseRefresh(s); ok {
		return d
	}
	return defaultRefresh
}

// ranges returns ranges within retained history, or the shortest range.
func (a *App) ranges() []time.Duration {
	n := 1
	for n < len(timeRanges) && timeRanges[n] <= a.market.History() {
		n++
	}
	return timeRanges[:n]
}

func (a *App) parseRange(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && slices.Contains(a.ranges(), d)
}

// rangeOf uses the default or longest retained range for unsupported URL values.
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

// rangeName omits zero minute and second components from URL values.
func rangeName(d time.Duration) string {
	if d%time.Hour == 0 {
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	}
	return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
}

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

// parseIndexes returns known symbols in sidebar order and defaults an empty value.
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
