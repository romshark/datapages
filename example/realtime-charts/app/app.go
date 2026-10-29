package app

import (
	"embed"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/realtime-charts/market"
	"github.com/romshark/datapages/example/realtime-charts/tsdb"
)

// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS

// EventIndexTicked is "index.ticked"
//
// The market loop publishes 1 event per index after each tick.
// Streams read prices from the database.
type EventIndexTicked struct {
	Index datapages.Subject `json:"index"`
}

// EventSettingsChanged is "settings.changed"
//
// The settings actions dispatch it after updating a tab's state.
type EventSettingsChanged struct {
	Tab datapages.SubjectStateID `json:"tab"`
}

type App struct {
	db     *tsdb.DB
	market *market.Market
}

// New returns an app that shows the prices m writes into db.
func New(db *tsdb.DB, m *market.Market) *App { return &App{db: db, market: m} }

func (*App) Head(r *http.Request) datapages.Head { return head() }

func (*App) RecoverError(err error, sse datapages.SSE) error {
	msg := "Something went wrong. Please try again."
	if errors.Is(err, datapages.ErrBadRequest) {
		msg = "The server rejected these settings."
	}
	return sse.PatchElement(errorMessage(msg))
}

// POSTSettings is /settings/{$}
//
// Updates the calling tab and dispatches [EventSettingsChanged] because an App action
// cannot patch its originating page.
func (a *App) POSTSettings(
	r *http.Request,
	state datapages.State[StateCharts],
	stateID string,
	signals datapages.Signals[Settings],
	settingsChanged datapages.Dispatcher[EventSettingsChanged],
) error {
	typ, ok := parseType(signals.Values.Type)
	if !ok {
		return fmt.Errorf("%w: chart type %q",
			datapages.ErrBadRequest, signals.Values.Type)
	}
	lay, ok := parseLayout(signals.Values.Layout)
	if !ok {
		return fmt.Errorf("%w: layout %q",
			datapages.ErrBadRequest, signals.Values.Layout)
	}
	refresh, ok := parseRefresh(signals.Values.Refresh)
	if !ok {
		return fmt.Errorf("%w: refresh interval %q",
			datapages.ErrBadRequest, signals.Values.Refresh)
	}
	rng, ok := a.parseRange(signals.Values.Range)
	if !ok {
		return fmt.Errorf("%w: time range %q",
			datapages.ErrBadRequest, signals.Values.Range)
	}
	paused := a.pausedAt(signals.Values.Paused)
	state.Values.Type = typ
	state.Values.Refresh = refresh
	state.Values.Range = rng
	state.Values.Resumed = state.Values.Paused != 0 && paused == 0
	state.Values.Paused = paused
	state.Values.Sync = signals.Values.Sync
	state.Values.Layout = lay
	return settingsChanged.Dispatch(EventSettingsChanged{
		Tab: datapages.SubjectStateID(stateID),
	})
}

// POSTToggle is /toggle/{symbol}/{$}
func (a *App) POSTToggle(
	r *http.Request,
	path datapages.Path[struct {
		Symbol string `path:"symbol"`
	}],
	state datapages.State[StateCharts],
	stateID string,
	settingsChanged datapages.Dispatcher[EventSettingsChanged],
) error {
	if _, ok := position[path.Values.Symbol]; !ok {
		return fmt.Errorf("%w: index %q", datapages.ErrNotFound, path.Values.Symbol)
	}
	state.Values.Indexes = toggled(state.Values.Indexes, path.Values.Symbol)
	return settingsChanged.Dispatch(EventSettingsChanged{
		Tab: datapages.SubjectStateID(stateID),
	})
}

// POSTSelect is /select/{selection}/{$}
func (a *App) POSTSelect(
	r *http.Request,
	path datapages.Path[struct {
		Selection string `path:"selection"`
	}],
	state datapages.State[StateCharts],
	stateID string,
	settingsChanged datapages.Dispatcher[EventSettingsChanged],
) error {
	switch path.Values.Selection {
	case "all":
		// The state must not share everyIndex with every other tab.
		state.Values.Indexes = slices.Clone(everyIndex)
	case "none":
		state.Values.Indexes = nil
	default:
		return fmt.Errorf("%w: selection %q",
			datapages.ErrNotFound, path.Values.Selection)
	}
	return settingsChanged.Dispatch(EventSettingsChanged{
		Tab: datapages.SubjectStateID(stateID),
	})
}

func (a *App) now() int64 { return a.market.Now() / 1000 }

// pausedAt clamps p to the retained market seconds. Zero keeps the tab live.
func (a *App) pausedAt(p int64) int64 {
	if p <= 0 {
		return 0
	}
	// Every index has the same timestamps, so the first represents all of them.
	first, _ := a.db.First(market.Indexes[0].Symbol)
	return min(max(p, (first.Time+999)/1000), a.now())
}

func (a *App) shownAt(paused int64) int64 {
	if paused != 0 {
		return paused
	}
	return a.now()
}

var position = func() map[string]int {
	m := make(map[string]int, len(market.Indexes))
	for i, in := range market.Indexes {
		m[in.Symbol] = i
	}
	return m
}()

var everyIndex = func() []string {
	symbols := make([]string, len(market.Indexes))
	for i, in := range market.Indexes {
		symbols[i] = in.Symbol
	}
	return symbols
}()

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return pageError("Page not found",
		"There is nothing at this address."), nil
}

// PageError500 is /error
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return pageError("Something went wrong",
		"The server failed to render this page."), nil
}
