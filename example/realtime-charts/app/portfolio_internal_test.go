package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBinWidth tests round widths capped at 8 positive bins.
func TestBinWidth(t *testing.T) {
	for extreme, want := range map[float64]float64{
		0:    0.01,
		0.07: 0.01,
		0.37: 0.05,
		0.9:  0.2,
		3:    0.5,
		7:    1,
	} {
		require.InDelta(t, want, binWidth(extreme), 1e-12, "extreme %v", extreme)
	}
}

// TestDistribution tests consecutive returns by bin center.
func TestDistribution(t *testing.T) {
	require.Nil(t, distribution([]float64{100}))
	require.Equal(t, []point{{0, 2}}, distribution([]float64{100, 100, 100}))

	// +25% and -20% in bins of width 5 from -25 to 25.
	rows := distribution([]float64{100, 125, 100})
	require.Len(t, rows, 11)
	for i, row := range rows {
		want := 0.0
		if i == 1 || i == 10 {
			want = 1
		}
		require.Equal(t, point{float64(i-5) * 5, want}, row, "bin %d", i)
	}
}

// TestSectorShares tests sector order, percentages and ring gaps.
func TestSectorShares(t *testing.T) {
	shares := sectorShares([]string{"VEIL", "BLTX", "AURA", "GLWT"})
	require.Len(t, shares, 2)
	for i, want := range []struct {
		sector string
		color  int
		offset float64
	}{{"Technology", 0, 0}, {"Energy", 3, 50}} {
		require.Equal(t, want.sector, shares[i].Sector)
		require.Equal(t, want.color, shares[i].Color)
		require.InDelta(t, 50, shares[i].Percent, 1e-9)
		require.InDelta(t, want.offset, shares[i].Offset, 1e-9)
		require.InDelta(t, 49.2, shares[i].Dash, 1e-9)
	}

	// A single sector is the whole ring.
	require.Equal(t, []share{{"Health", 5, 100, 0, 100}}, sectorShares([]string{"CRUX"}))
}

// TestDollars tests rounding, separators and negative zero.
func TestDollars(t *testing.T) {
	for v, want := range map[float64]string{
		102837.4:  "$102,837",
		1234567.5: "$1,234,568",
		999.4:     "$999",
		0:         "$0",
		-0.4:      "$0",
		-1234.6:   "-$1,235",
	} {
		require.Equal(t, want, dollars(v), "%v", v)
	}
}
