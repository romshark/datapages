package app

import (
	"embed"
	"errors"
	"fmt"
	"net/http"

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
// The market loop in cmd/server publishes it for every index after every
// tick, each index on a subject of its own: index.ticked.GLWT for GLWT.
// The event carries nothing else: a stream reads the prices from the database.
type EventIndexTicked struct {
	Index datapages.Subject `json:"index"`
}

// EventSettingsChanged is "settings.changed"
//
// POSTSettings and POSTToggle dispatch it to the tab whose settings they
// changed. The settings are in the state of that tab.
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
// It changes the refresh interval of the calling tab and pauses or resumes
// it. Every view page calls it, which makes it an action of App, and an App
// action cannot patch the page that called it. It dispatches an event to the
// calling tab instead, and [Dashboard.OnSettingsChanged] renders the change
// on the stream of that tab.
func (a *App) POSTSettings(
	r *http.Request,
	state datapages.State[StateCharts],
	stateID string,
	signals datapages.Signals[Settings],
	settingsChanged datapages.Dispatcher[EventSettingsChanged],
) error {
	refresh, ok := parseRefresh(signals.Values.Refresh)
	if !ok {
		return fmt.Errorf("%w: refresh interval %q",
			datapages.ErrBadRequest, signals.Values.Refresh)
	}
	paused := a.pausedAt(signals.Values.Paused)
	state.Values.Refresh = refresh
	state.Values.Resumed = state.Values.Paused != 0 && paused == 0
	state.Values.Paused = paused
	return settingsChanged.Dispatch(EventSettingsChanged{
		Tab: datapages.SubjectStateID(stateID),
	})
}

// POSTToggle is /toggle/{symbol}/{$}
//
// It shows or hides the chart of an index on the calling tab, the way
// POSTSettings changes the settings.
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

// now returns the time of the newest prices in Unix seconds.
func (a *App) now() int64 { return a.market.Now() / 1000 }

// pausedAt returns the Unix second p freezes the charts at: zero for a live
// tab, else p moved into the seconds the database holds. A link to a moment
// the database no longer holds shows the oldest one it does.
func (a *App) pausedAt(p int64) int64 {
	if p <= 0 {
		return 0
	}
	// Every index has a price at the same times, which makes the first
	// index stand for all of them.
	first, _ := a.db.First(market.Indexes[0].Symbol)
	return min(max(p, (first.Time+999)/1000), a.now())
}

// position maps the symbol of an index to its position in [market.Indexes].
var position = func() map[string]int {
	m := make(map[string]int, len(market.Indexes))
	for i, in := range market.Indexes {
		m[in.Symbol] = i
	}
	return m
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
