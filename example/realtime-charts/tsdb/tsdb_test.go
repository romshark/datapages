package tsdb_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/realtime-charts/tsdb"
)

// TestAppend tests that Append keeps the order of a series and adds nothing
// when a point is not newer than the one before it.
func TestAppend(t *testing.T) {
	for name, tc := range map[string]struct {
		points []tsdb.Point
		err    error
		want   []tsdb.Point
	}{
		"newer points": {
			points: []tsdb.Point{{4, 4}, {5, 5}},
			want:   []tsdb.Point{{1, 1}, {2, 2}, {4, 4}, {5, 5}},
		},
		"same time as the last": {
			points: []tsdb.Point{{2, 9}},
			err:    tsdb.ErrOutOfOrder,
			want:   []tsdb.Point{{1, 1}, {2, 2}},
		},
		"out of order within the call": {
			points: []tsdb.Point{{4, 4}, {3, 3}},
			err:    tsdb.ErrOutOfOrder,
			want:   []tsdb.Point{{1, 1}, {2, 2}},
		},
		"nothing": {
			want: []tsdb.Point{{1, 1}, {2, 2}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := tsdb.New(time.Hour)
			require.NoError(t, db.Append("a", tsdb.Point{1, 1}, tsdb.Point{2, 2}))
			require.ErrorIs(t, db.Append("a", tc.points...), tc.err)
			require.Equal(t, tc.want, db.Range("a", 0, 100))
		})
	}
}

// TestRetention tests that a series keeps the points of the retention
// before its newest point.
func TestRetention(t *testing.T) {
	db := tsdb.New(10 * time.Millisecond)
	for ts := int64(1); ts <= 30; ts++ {
		require.NoError(t, db.Append("a", tsdb.Point{Time: ts, Value: float64(ts)}))
	}
	first, ok := db.First("a")
	require.True(t, ok)
	require.Equal(t, tsdb.Point{Time: 20, Value: 20}, first)
	require.Len(t, db.Range("a", 0, 100), 11)
}

// TestRange tests the bounds of a range and that it returns a copy.
func TestRange(t *testing.T) {
	db := tsdb.New(time.Hour)
	require.NoError(t, db.Append("a",
		tsdb.Point{10, 1}, tsdb.Point{20, 2}, tsdb.Point{30, 3}))
	for name, tc := range map[string]struct {
		from, to int64
		want     []tsdb.Point
	}{
		"inclusive": {from: 10, to: 30, want: []tsdb.Point{{10, 1}, {20, 2}, {30, 3}}},
		"inside":    {from: 11, to: 29, want: []tsdb.Point{{20, 2}}},
		"before":    {from: 0, to: 9, want: []tsdb.Point{}},
		"reversed":  {from: 30, to: 10, want: []tsdb.Point{}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, db.Range("a", tc.from, tc.to))
		})
	}
	db.Range("a", 0, 100)[0].Value = 99
	require.Equal(t, 1.0, db.Range("a", 0, 100)[0].Value)
	require.Empty(t, db.Range("missing", 0, 100))
}

// TestAt tests which point At, First and Last return.
func TestAt(t *testing.T) {
	db := tsdb.New(time.Hour)
	require.NoError(t, db.Append("a", tsdb.Point{10, 1}, tsdb.Point{20, 2}))
	for name, tc := range map[string]struct {
		t    int64
		want tsdb.Point
		ok   bool
	}{
		"before the first": {t: 9},
		"at a point":       {t: 10, want: tsdb.Point{10, 1}, ok: true},
		"between points":   {t: 19, want: tsdb.Point{10, 1}, ok: true},
		"after the last":   {t: 99, want: tsdb.Point{20, 2}, ok: true},
	} {
		t.Run(name, func(t *testing.T) {
			p, ok := db.At("a", tc.t)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, p)
		})
	}
	first, _ := db.First("a")
	last, _ := db.Last("a")
	require.Equal(t, tsdb.Point{10, 1}, first)
	require.Equal(t, tsdb.Point{20, 2}, last)
	_, ok := db.Last("missing")
	require.False(t, ok)
}

// TestOHLC tests that the buckets start at multiples of the step, carry the
// open, high, low and close of their points and leave out empty steps.
func TestOHLC(t *testing.T) {
	db := tsdb.New(time.Hour)
	require.NoError(t, db.Append("a",
		tsdb.Point{1000, 5}, tsdb.Point{1250, 7}, tsdb.Point{1500, 4}, tsdb.Point{1750, 6},
		tsdb.Point{2000, 6},
		tsdb.Point{4250, 3},
	))
	require.Equal(t, []tsdb.Bucket{
		{Time: 1000, Open: 5, High: 7, Low: 4, Close: 6},
		{Time: 2000, Open: 6, High: 6, Low: 6, Close: 6},
		{Time: 4000, Open: 3, High: 3, Low: 3, Close: 3},
	}, db.OHLC("a", 0, 5000, 1000))

	// A range that ends inside a bucket shows the bucket as it was then.
	require.Equal(t, []tsdb.Bucket{
		{Time: 1000, Open: 5, High: 7, Low: 5, Close: 7},
	}, db.OHLC("a", 0, 1250, 1000))
}
