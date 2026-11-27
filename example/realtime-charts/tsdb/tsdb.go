// Package tsdb stores retained time series in memory and aggregates OHLC buckets.
package tsdb

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

// Point is a value at a time in Unix milliseconds.
type Point struct {
	Time  int64
	Value float64
}

// Bucket is the open, high, low and close of 1 step.
type Bucket struct {
	// Time is the start of the bucket in Unix milliseconds, a multiple of its step.
	Time                   int64
	Open, High, Low, Close float64
}

// ErrOutOfOrder reports a point that is not newer than the one before it.
var ErrOutOfOrder = errors.New("point not newer than the one before it")

// DB is safe for concurrent use.
type DB struct {
	retention int64

	mu     sync.RWMutex
	series map[string][]Point
}

// New returns an empty database with the given per-series retention.
func New(retention time.Duration) *DB {
	return &DB{retention: retention.Milliseconds(), series: map[string][]Point{}}
}

// Append adds points oldest first and creates the series.
// Out-of-order input adds nothing and returns [ErrOutOfOrder].
// Points beyond retention are dropped.
func (db *DB) Append(name string, points ...Point) error {
	if len(points) == 0 {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	s := db.series[name]
	var prev int64
	have := len(s) > 0
	if have {
		prev = s[len(s)-1].Time
	}
	for _, p := range points {
		if have && p.Time <= prev {
			return fmt.Errorf("%w: %s at %d", ErrOutOfOrder, name, p.Time)
		}
		prev, have = p.Time, true
	}
	s = append(s, points...)
	// Reslicing avoids a copy. A later append frees the skipped prefix when it grows.
	newest := s[len(s)-1].Time
	s = s[sort.Search(len(s), func(i int) bool {
		return s[i].Time >= newest-db.retention
	}):]
	db.series[name] = s
	return nil
}

// Range returns the points of name with from <= Time <= to, oldest first.
func (db *DB) Range(name string, from, to int64) []Point {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return slices.Clone(within(db.series[name], from, to))
}

// At returns the newest point of name at or before t.
func (db *DB) At(name string, t int64) (Point, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s := db.series[name]
	i := sort.Search(len(s), func(i int) bool { return s[i].Time > t })
	if i == 0 {
		return Point{}, false
	}
	return s[i-1], true
}

// First returns the oldest point of name the database keeps.
func (db *DB) First(name string) (Point, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s := db.series[name]
	if len(s) == 0 {
		return Point{}, false
	}
	return s[0], true
}

// Last returns the newest point of name.
func (db *DB) Last(name string) (Point, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s := db.series[name]
	if len(s) == 0 {
		return Point{}, false
	}
	return s[len(s)-1], true
}

// OHLC aggregates from <= Time <= to into aligned
// step-millisecond buckets, oldest first. Empty steps have no bucket.
func (db *DB) OHLC(name string, from, to, step int64) []Bucket {
	db.mu.RLock()
	defer db.mu.RUnlock()
	var buckets []Bucket
	for _, p := range within(db.series[name], from, to) {
		start := p.Time - p.Time%step
		if n := len(buckets); n > 0 && buckets[n-1].Time == start {
			b := &buckets[n-1]
			b.High = max(b.High, p.Value)
			b.Low = min(b.Low, p.Value)
			b.Close = p.Value
			continue
		}
		buckets = append(buckets, Bucket{
			Time: start, Open: p.Value, High: p.Value, Low: p.Value, Close: p.Value,
		})
	}
	return buckets
}

func within(s []Point, from, to int64) []Point {
	i := sort.Search(len(s), func(i int) bool { return s[i].Time >= from })
	j := sort.Search(len(s), func(i int) bool { return s[i].Time > to })
	return s[i:max(i, j)]
}
