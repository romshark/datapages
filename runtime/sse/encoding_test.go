package sse

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestEmptyStreams tests that each coding's empty stream decodes to no data.
func TestEmptyStreams(t *testing.T) {
	for name, tc := range map[string]struct {
		empty  []byte
		decode func(io.Reader) (io.Reader, error)
	}{
		"zstd": {emptyZstd, func(r io.Reader) (io.Reader, error) {
			d, err := zstd.NewReader(r)
			if err != nil {
				return nil, err
			}
			t.Cleanup(d.Close)
			return d, nil
		}},
		"br": {emptyBrotli, func(r io.Reader) (io.Reader, error) {
			return brotli.NewReader(r), nil
		}},
		"gzip": {emptyGzip, func(r io.Reader) (io.Reader, error) {
			return gzip.NewReader(r)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := tc.decode(bytes.NewReader(tc.empty))
			require.NoError(t, err)
			out, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Empty(t, out)
		})
	}
}

// TestEmptyGzip tests that [gzip.Writer] produces the stored empty stream.
func TestEmptyGzip(t *testing.T) {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	require.NoError(t, zw.Close())
	require.Equal(t, b.Bytes(), emptyGzip)
}

// TestNegotiate tests Accept-Encoding selection with the default codings.
func TestNegotiate(t *testing.T) {
	for name, tc := range map[string]struct {
		values []string
		want   string // "" for no compression.
	}{
		"none":                      {nil, ""},
		"empty":                     {[]string{""}, ""},
		"identity":                  {[]string{"identity"}, ""},
		"deflate only":              {[]string{"deflate"}, ""},
		"browser":                   {[]string{"gzip, deflate, br, zstd"}, "zstd"},
		"without zstd":              {[]string{"gzip, deflate, br"}, "br"},
		"gzip only":                 {[]string{"gzip"}, "gzip"},
		"zstd preferred over order": {[]string{"br;q=1.0, zstd;q=0.1"}, "zstd"},
		"upper case":                {[]string{"ZSTD"}, "zstd"},
		"x-gzip":                    {[]string{"x-gzip"}, "gzip"},
		"zstd refused":              {[]string{"zstd;q=0, br, gzip"}, "br"},
		"zstd and br refused":       {[]string{"zstd;q=0, br;q=0, gzip"}, "gzip"},
		"all refused":               {[]string{"zstd;q=0, br;q=0, gzip;q=0"}, ""},
		"quality zero decimals":     {[]string{"zstd; q=0.000, gzip"}, "gzip"},
		"quality zero dot":          {[]string{"zstd;q=0., gzip"}, "gzip"},
		"quality 0.001":             {[]string{"zstd;q=0.001"}, "zstd"},
		"quality with spaces":       {[]string{"zstd ; Q = 0 , gzip"}, "gzip"},
		"other parameter":           {[]string{"zstd;level=0"}, "zstd"},
		"first mention counts":      {[]string{"zstd;q=0, zstd"}, ""},
		"star":                      {[]string{"*"}, "gzip"},
		"star refused":              {[]string{"*;q=0"}, ""},
		"gzip refused before star":  {[]string{"gzip;q=0, *"}, ""},
		"star refused, gzip listed": {[]string{"*;q=0, gzip"}, "gzip"},
		"second header":             {[]string{"gzip", "zstd"}, "zstd"},
	} {
		t.Run(name, func(t *testing.T) {
			got := ""
			if c := NewCompression(nil).negotiate(tc.values); c != nil {
				got = c.name
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestNegotiateCodings tests Accept-Encoding selection with the codings of
// [datapages.SSECompressionConfig.Codings], in their order.
func TestNegotiateCodings(t *testing.T) {
	const (
		zstd   = datapages.SSECodingZstd
		brotli = datapages.SSECodingBrotli
		gzip   = datapages.SSECodingGzip
	)
	for name, tc := range map[string]struct {
		codings []datapages.SSECoding
		values  []string
		want    string // "" for no compression.
	}{
		"gzip first": {
			[]datapages.SSECoding{gzip, zstd, brotli},
			[]string{"zstd, br, gzip"},
			"gzip",
		},
		"brotli first": {
			[]datapages.SSECoding{brotli, zstd},
			[]string{"zstd, br"},
			"br",
		},
		"first coding refused": {
			[]datapages.SSECoding{gzip, zstd},
			[]string{"gzip;q=0, zstd"},
			"zstd",
		},
		"none accepted": {
			[]datapages.SSECoding{zstd},
			[]string{"gzip, br"},
			"",
		},
		"star with gzip": {
			[]datapages.SSECoding{zstd, gzip},
			[]string{"*"},
			"gzip",
		},
		"star without gzip": {
			[]datapages.SSECoding{zstd, brotli},
			[]string{"*"},
			"",
		},
		"star after listed": {
			[]datapages.SSECoding{brotli, gzip},
			[]string{"br;q=0, *"},
			"gzip",
		},
		"gzip refused by star": {
			[]datapages.SSECoding{gzip},
			[]string{"gzip;q=0, *"},
			"",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := NewCompression(&datapages.SSECompressionConfig{Codings: tc.codings})
			got := ""
			if cd := c.negotiate(tc.values); cd != nil {
				got = cd.name
			}
			require.Equal(t, tc.want, got)
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
