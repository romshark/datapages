package market_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/realtime-charts/market"
	"github.com/romshark/datapages/example/realtime-charts/tsdb"
)

// t0 starts each test 2 seconds into a 5-second bar.
var t0 = time.Unix(1_800_000_002, 0)

const history = time.Hour

func newMarket() (*market.Market, *tsdb.DB) {
	db := tsdb.New(history)
	return market.New(db, t0, history), db
}

// TestNew tests history prefill.
func TestNew(t *testing.T) {
	m, db := newMarket()
	require.Equal(t, t0.UnixMilli(), m.Now())
	for i, in := range market.Indexes {
		points := db.Range(in.Symbol, 0, t0.UnixMilli())
		require.Len(t, points, int(history.Milliseconds()/market.Step)+1, in.Symbol)
		require.Equal(t, t0.Add(-history).UnixMilli(), points[0].Time, in.Symbol)
		require.Equal(t, t0.UnixMilli(), points[len(points)-1].Time, in.Symbol)
		require.InEpsilon(t, m.Open(i), points[0].Value, 0.05, in.Symbol)
	}
}

// TestNewIsDeterministic tests equal seeds at the same millisecond.
func TestNewIsDeterministic(t *testing.T) {
	_, a := newMarket()
	_, b := newMarket()
	for _, in := range market.Indexes {
		require.Equal(t,
			a.Range(in.Symbol, 0, t0.UnixMilli()), b.Range(in.Symbol, 0, t0.UnixMilli()),
			in.Symbol)
	}
}

// TestTick tests which prices a tick writes.
func TestTick(t *testing.T) {
	for name, tc := range map[string]struct {
		at    time.Time
		added int // points per index after t0
	}{
		"same second":    {at: t0},
		"earlier second": {at: t0.Add(-3 * time.Second)},
		"inside a step":  {at: t0.Add(100 * time.Millisecond)},
		"one step":       {at: t0.Add(250 * time.Millisecond), added: 1},
		"one second":     {at: t0.Add(time.Second), added: 4},
		"half an hour":   {at: t0.Add(30 * time.Minute), added: 7200},
		// Only the retained hour is simulated after the gap.
		"longer than the history": {
			at:    t0.Add(10 * time.Hour),
			added: int(history.Milliseconds()/market.Step) + 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, db := newMarket()
			m.Tick(tc.at)
			newest := max(
				t0.UnixMilli(),
				tc.at.UnixMilli()-tc.at.UnixMilli()%market.Step,
			)
			require.Equal(t, newest, m.Now())
			for _, in := range market.Indexes {
				added := db.Range(in.Symbol, t0.UnixMilli()+1, newest)
				require.Len(t, added, tc.added, in.Symbol)
				for i, p := range added {
					require.Zero(t, p.Time%market.Step, in.Symbol)
					if i > 0 {
						require.Equal(t, added[i-1].Time+market.Step, p.Time, in.Symbol)
					}
				}
				first, _ := db.First(in.Symbol)
				require.GreaterOrEqual(t,
					first.Time,
					newest-history.Milliseconds(), in.Symbol)
			}
		})
	}
}

// TestSectors tests complete bidirectional membership.
func TestSectors(t *testing.T) {
	counts := map[string]int{}
	for _, in := range market.Indexes {
		require.Contains(t, market.Sectors, in.Sector, in.Symbol)
		counts[in.Sector]++
	}
	for _, sector := range market.Sectors {
		require.Positive(t, counts[sector], sector)
	}
}
