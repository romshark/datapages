package levels_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/contentcoding/levels"
)

// TestCheck tests the bounds of each coding.
func TestCheck(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		check   func(int) error
		level   int
		wantErr string
	}{
		"default":        {check: levels.Zstd},
		"zstd highest":   {check: levels.Zstd, level: 22},
		"brotli highest": {check: levels.Brotli, level: 11},
		"gzip lowest":    {check: levels.Gzip, level: 1},

		"zstd too high": {
			check: levels.Zstd, level: 23,
			wantErr: "zstd level (23) must be from 1 to 22",
		},
		"brotli too high": {
			check: levels.Brotli, level: 12,
			wantErr: "brotli quality (12) must be from 1 to 11",
		},
		"gzip negative": {
			check: levels.Gzip, level: -1,
			wantErr: "gzip level (-1) must be from 1 to 9",
		},
	} {
		err := tc.check(tc.level)
		if tc.wantErr == "" {
			require.NoError(t, err, name)
			continue
		}
		require.EqualError(t, err, tc.wantErr, name)
	}
}
