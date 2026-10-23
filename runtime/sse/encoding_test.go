package sse

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestNegotiateCodings tests that [datapages.SSECompressionConfig.Codings]
// sets the codings a stream may use and their order.
func TestNegotiateCodings(t *testing.T) {
	const (
		zstd   = datapages.CodingZstd
		brotli = datapages.CodingBrotli
		gzip   = datapages.CodingGzip
	)
	for name, tc := range map[string]struct {
		codings []datapages.Coding
		values  []string
		want    string // "" for no compression.
	}{
		"gzip first": {
			[]datapages.Coding{gzip, zstd, brotli},
			[]string{"zstd, br, gzip"},
			"gzip",
		},
		"brotli first": {
			[]datapages.Coding{brotli, zstd},
			[]string{"zstd, br"},
			"br",
		},
		"none accepted": {
			[]datapages.Coding{zstd},
			[]string{"gzip, br"},
			"",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := NewCompression(&datapages.SSECompressionConfig{Codings: tc.codings})
			got := ""
			if cd := c.negotiate(tc.values); cd != nil {
				got = cd.Name()
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCompressionPresets tests the coding each preset selects for a browser and
// the settings it resolves to.
func TestCompressionPresets(t *testing.T) {
	for name, tc := range map[string]struct {
		conf   datapages.SSECompressionConfig
		coding string
		key    string
	}{
		"balanced": {
			datapages.SSECompressionBalanced(), "zstd",
			"zstd,br,gzip,1,1048576,4,1048576,6",
		},
		"low memory": {
			datapages.SSECompressionLowMemory(), "zstd",
			"zstd,br,gzip,1,262144,4,262144,6",
		},
		"low bandwidth": {
			datapages.SSECompressionLowBandwidth(), "br",
			"br,zstd,gzip,1,1048576,4,1048576,6",
		},
		"large fragments": {
			datapages.SSECompressionLargeFragments(), "zstd",
			"zstd,br,gzip,1,4194304,4,4194304,6",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cd := NewCompression(&tc.conf).negotiate([]string{"gzip, deflate, br, zstd"})
			require.NotNil(t, cd)
			require.Equal(t, tc.coding, cd.Name())
			require.Equal(t, tc.key, CompressionKey(&tc.conf))
		})
	}
}

// TestNewCompressionDisabled tests that a disabled configuration compresses
// nothing.
func TestNewCompressionDisabled(t *testing.T) {
	c := NewCompression(&datapages.SSECompressionConfig{Disabled: true})
	require.Nil(t, c)
	require.Nil(t, c.negotiate([]string{"gzip, deflate, br, zstd"}))
}
