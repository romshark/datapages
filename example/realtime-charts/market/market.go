// Package market simulates fictional indexes in a time-series database.
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

// Index names a fictional stock index and its sector, one of [Sectors].
type Index struct {
	Symbol string
	Name   string
	Sector string
}

// Market simulates [Indexes] in a database. It's safe for concurrent use.
type Market struct {
	db      *tsdb.DB
	history int64

	mu    sync.Mutex
	now   int64
	walks []walk
}

type walk struct {
	rng      *rand.Rand
	open     float64
	logPrice float64
	logMean  float64
	sigma    float64
}

// New fills db at every Step through now.
// The same millisecond produces the same prices.
func New(db *tsdb.DB, now time.Time, history time.Duration) *Market {
	m := &Market{
		db:      db,
		history: history.Milliseconds(),
		walks:   make([]walk, len(Indexes)),
	}
	for i := range Indexes {
		// Per-index generators make prices independent of the number of indexes.
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

// History returns the configured history.
func (m *Market) History() time.Duration {
	return time.Duration(m.history) * time.Millisecond
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

// Tick writes every Step through now. A tick at or before the last one writes nothing.
//
// A gap longer than History skips expired steps but continues the prior prices.
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
		// These times are newer than every existing point, which is all Append checks.
		_ = m.db.Append(Indexes[i].Symbol, points...)
	}
	m.now = t
}

// Run ticks at each full second and notifies every index until ctx ends.
func (m *Market) Run(ctx context.Context, notify func(symbol string)) {
	// Resetting to each full second avoids duplicate
	// or skipped Unix seconds after delays.
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

func (w *walk) step() float64 {
	// Mean reversion keeps a long-running log-price walk away from 0 and infinity.
	w.logPrice += 1e-4*(w.logMean-w.logPrice) + w.sigma*w.rng.NormFloat64()
	return math.Exp(w.logPrice)
}
