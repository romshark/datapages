// Package market simulates the prices of fictional indexes and writes them
// into a time-series database, one series per index.
package market

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/romshark/datapages/example/realtime-charts/tsdb"
)

// Step is the time between two prices of an index in milliseconds.
const Step = 250

// Index is a fictional stock index.
type Index struct {
	Symbol string
	Name   string
}

// Market simulates the prices of [Indexes] and writes them into a database.
// It is safe for concurrent use.
type Market struct {
	db      *tsdb.DB
	history int64

	mu    sync.Mutex
	now   int64
	walks []walk
}

// walk is the price process of one index.
type walk struct {
	rng      *rand.Rand
	open     float64
	logPrice float64
	logMean  float64
	sigma    float64
}

// New returns a market whose indexes have a price in db for every Step of
// the history before now. Markets created at the same millisecond write the
// same prices.
func New(db *tsdb.DB, now time.Time, history time.Duration) *Market {
	m := &Market{
		db:      db,
		history: history.Milliseconds(),
		walks:   make([]walk, len(Indexes)),
	}
	for i := range Indexes {
		// Each index has its own generator, which keeps its prices
		// independent of how many other indexes the market holds.
		rng := rand.New(rand.NewPCG(uint64(i)+1, 0x9e3779b97f4a7c15))
		price := math.Round((20+rng.Float64()*380)*100) / 100
		m.walks[i] = walk{
			rng:      rng,
			open:     price,
			logPrice: math.Log(price),
			logMean:  math.Log(price),
			sigma:    0.0004 + rng.Float64()*0.0012,
		}
	}
	t := now.UnixMilli()
	m.now = t - t%Step - m.history - Step
	m.Tick(now)
	return m
}

// Now returns the time of the newest prices in Unix milliseconds.
func (m *Market) Now() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Open returns the first price of the index at position i of [Indexes].
func (m *Market) Open(i int) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.walks[i].open
}

// Tick writes a price for every index and every Step from the last tick to
// now. A tick at or before the last one writes nothing.
//
// A gap longer than the history, such as after the host was suspended, is
// not simulated step by step: prices continue from where they were and the
// database drops the points before the gap.
func (m *Market) Tick(now time.Time) {
	t := now.UnixMilli()
	t -= t % Step
	m.mu.Lock()
	defer m.mu.Unlock()
	if t <= m.now {
		return
	}
	from := max(m.now+Step, t-m.history)
	points := make([]tsdb.Point, 0, (t-from)/Step+1)
	for i := range m.walks {
		points = points[:0]
		for ts := from; ts <= t; ts += Step {
			points = append(points, tsdb.Point{Time: ts, Value: m.walks[i].step()})
		}
		// The times are newer than every point written before, which is
		// all Append checks.
		_ = m.db.Append(Indexes[i].Symbol, points...)
	}
	m.now = t
}

// Run ticks the market at the start of every second until ctx is done.
// After each tick it calls notify with the symbol of every index.
func (m *Market) Run(ctx context.Context, notify func(symbol string)) {
	// A timer reset to the next full second fires once per Unix second,
	// where a ticker started at an arbitrary offset can fire twice within one
	// second and skip the next when it runs late.
	next := func(t time.Time) time.Duration {
		return time.Until(t.Truncate(time.Second).Add(time.Second))
	}
	timer := time.NewTimer(next(time.Now()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			m.Tick(now)
			for _, in := range Indexes {
				notify(in.Symbol)
			}
			timer.Reset(next(now))
		}
	}
}

// step moves the price one Step and returns it.
func (w *walk) step() float64 {
	// An Ornstein-Uhlenbeck process on the log price. The pull towards the
	// opening price keeps a market that runs for days in a plausible range,
	// where a plain random walk would drift towards zero or infinity.
	w.logPrice += 1e-4*(w.logMean-w.logPrice) + w.sigma*w.rng.NormFloat64()
	return math.Exp(w.logPrice)
}
