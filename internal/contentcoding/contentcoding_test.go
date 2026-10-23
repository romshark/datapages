package contentcoding

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func decoder(t *testing.T, name string, r io.Reader) io.Reader {
	t.Helper()
	switch name {
	case "zstd":
		d, err := zstd.NewReader(r)
		require.NoError(t, err)
		t.Cleanup(d.Close)
		return d
	case "br":
		return brotli.NewReader(r)
	case "gzip":
		zr, err := gzip.NewReader(r)
		require.NoError(t, err)
		return zr
	}
	t.Fatalf("unknown coding %q", name)
	return nil
}

// TestEmptyStreams tests that each stored empty stream decodes to no data.
func TestEmptyStreams(t *testing.T) {
	for name, c := range map[string]*Coding{
		"zstd": NewZstd(1, 1<<20),
		"br":   NewBrotli(4, 1<<20),
		"gzip": NewGzip(6),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := io.ReadAll(decoder(t, name, bytes.NewReader(c.Empty())))
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

// TestEncoderReuse tests that a reused encoder contains only its current input.
func TestEncoderReuse(t *testing.T) {
	for name, c := range map[string]*Coding{
		"zstd": NewZstd(3, 256<<10),
		"br":   NewBrotli(5, 256<<10),
		"gzip": NewGzip(6),
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, name, c.Name())
			for i, in := range []string{
				strings.Repeat("<li>first</li>", 100),
				strings.Repeat("<li>second</li>", 50),
			} {
				var out bytes.Buffer
				e := c.Get(&out)
				_, err := io.WriteString(e, in)
				require.NoError(t, err)
				require.NoError(t, e.Close())
				c.Put(e)
				got, err := io.ReadAll(decoder(t, name, &out))
				require.NoError(t, err)
				require.Equal(t, in, string(got), "stream %d", i)
			}
		})
	}
}

// TestZstdLevel tests that each level maps to 1, 3, 6 or 10, the levels the docs
// of the ZstdLevel config fields list as the ones that differ.
func TestZstdLevel(t *testing.T) {
	for level, want := range map[int]int{
		1: 1, 2: 1,
		3: 3, 5: 3,
		6: 6, 9: 6,
		10: 10, 19: 10, 22: 10,
	} {
		require.Equal(t, want, ZstdLevel(level), "level %d", level)
	}
}

// TestNegotiateNoCodings tests that no codings select identity.
func TestNegotiateNoCodings(t *testing.T) {
	require.Nil(t, Negotiate(nil, []string{"gzip, deflate, br, zstd"}))
}

// TestNegotiate tests Accept-Encoding parsing with zstd, brotli and gzip
// in that order.
func TestNegotiate(t *testing.T) {
	codings := []*Coding{NewZstd(1, 1<<20), NewBrotli(4, 1<<20), NewGzip(6)}
	for name, tc := range map[string]struct {
		values []string
		want   string // "" for identity.
	}{
		"none":                      {nil, ""},
		"empty":                     {[]string{""}, ""},
		"identity":                  {[]string{"identity"}, ""},
		"identity refused":          {[]string{"identity;q=0, *;q=0"}, ""},
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
		"quality after a parameter": {[]string{"zstd;level=1;q=0, gzip"}, "gzip"},
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
			if c := Negotiate(codings, tc.values); c != nil {
				got = c.Name()
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestNegotiateOrder tests selection among codings in the order given, and
// that a wildcard selects gzip only when gzip is among them.
func TestNegotiateOrder(t *testing.T) {
	zstd, brotli, gzip := NewZstd(1, 1<<20), NewBrotli(4, 1<<20), NewGzip(6)
	for name, tc := range map[string]struct {
		codings []*Coding
		values  []string
		want    string // "" for identity.
	}{
		"gzip first": {
			[]*Coding{gzip, zstd, brotli}, []string{"zstd, br, gzip"}, "gzip",
		},
		"first coding refused": {
			[]*Coding{gzip, zstd}, []string{"gzip;q=0, zstd"}, "zstd",
		},
		"none accepted": {
			[]*Coding{zstd}, []string{"gzip, br"}, "",
		},
		"star with gzip": {
			[]*Coding{zstd, gzip}, []string{"*"}, "gzip",
		},
		"star without gzip": {
			[]*Coding{zstd, brotli}, []string{"*"}, "",
		},
		"star after listed": {
			[]*Coding{brotli, gzip}, []string{"br;q=0, *"}, "gzip",
		},
		"gzip refused by star": {
			[]*Coding{gzip}, []string{"gzip;q=0, *"}, "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ""
			if c := Negotiate(tc.codings, tc.values); c != nil {
				got = c.Name()
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCompressible tests which media types compression shrinks, parameters
// and case ignored.
func TestCompressible(t *testing.T) {
	for contentType, want := range map[string]bool{
		"text/html; charset=utf-8":  true,
		"TEXT/CSS":                  true,
		"application/json":          true,
		"application/ld+json":       true,
		"image/svg+xml":             true,
		"application/javascript":    true,
		"application/wasm":          true,
		" application/xml ; q=1 ":   true,
		"application/xhtml+xml;v=1": true,
		"application/yaml":          true,
		"application/x-yaml":        true,
		"application/toml":          true,
		"font/ttf":                  true,
		"font/otf":                  true,
		"font/collection":           true,
		"font/sfnt":                 true,
		"image/x-icon":              true,
		"image/vnd.microsoft.icon":  true,
		"image/bmp":                 true,

		"":                         false,
		"font/woff":                false,
		"font/woff2":               false,
		"text/event-stream":        false,
		"image/png":                false,
		"application/octet-stream": false,
		"application/json-seq":     false,
		"multipart/form-data; a=b": false,
		"image/webp":               false,
	} {
		require.Equal(t, want, Compressible(contentType), "%q", contentType)
	}
}

// TestTypeByExtension tests the font table in any case and the mime fallback.
func TestTypeByExtension(t *testing.T) {
	for ext, want := range map[string]string{
		".otf":     "font/otf",
		".OTF":     "font/otf",
		".ttf":     "font/ttf",
		".ttc":     "font/collection",
		".otc":     "font/collection",
		".woff":    "font/woff",
		".woff2":   "font/woff2",
		".css":     "text/css; charset=utf-8",
		".unknown": "",
	} {
		require.Equal(t, want, TypeByExtension(ext), ext)
	}
}

// TestNoTransform tests that the directive matches in any case, position and
// header value, and that longer names and a value don't.
func TestNoTransform(t *testing.T) {
	for name, tc := range map[string]struct {
		cacheControl []string
		want         bool
	}{
		"absent":          {nil, false},
		"other":           {[]string{"public, max-age=60"}, false},
		"alone":           {[]string{"no-transform"}, true},
		"case":            {[]string{"private, No-Transform"}, true},
		"spaces":          {[]string{"public ,  no-transform  , max-age=0"}, true},
		"in 2nd value":    {[]string{"public", "no-transform"}, true},
		"longer name":     {[]string{"no-transformation"}, false},
		"with a value":    {[]string{"no-transform=1"}, false},
		"in another name": {[]string{"x-no-transform"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			h := http.Header{"Cache-Control": tc.cacheControl}
			require.Equal(t, tc.want, NoTransform(h))
		})
	}
}

// TestAddVaryAcceptEncoding tests that Accept-Encoding is added once,
// whatever its case, and never next to *.
func TestAddVaryAcceptEncoding(t *testing.T) {
	for name, tc := range map[string]struct {
		vary []string
		want []string
	}{
		"none": {
			vary: nil,
			want: []string{"Accept-Encoding"},
		},
		"other": {
			vary: []string{"Cookie"},
			want: []string{"Cookie", "Accept-Encoding"},
		},
		"listed": {
			vary: []string{"Cookie, accept-encoding"},
			want: []string{"Cookie, accept-encoding"},
		},
		"in 2nd value": {
			vary: []string{"Cookie", "Accept-Encoding"},
			want: []string{"Cookie", "Accept-Encoding"},
		},
		"star": {[]string{"*"}, []string{"*"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tc.vary {
				h.Add("Vary", v)
			}
			AddVaryAcceptEncoding(h)
			require.Equal(t, tc.want, h.Values("Vary"))
		})
	}
}

// TestSharedValues tests that adding to a header leaves the values it shares
// with other headers intact.
func TestSharedValues(t *testing.T) {
	cd := NewGzip(1)
	first := http.Header{}
	AddVaryAcceptEncoding(first)
	cd.SetContentEncoding(first)
	first.Add("Vary", "Cookie")
	first.Add("Content-Encoding", "br")

	second := http.Header{}
	AddVaryAcceptEncoding(second)
	cd.SetContentEncoding(second)
	require.Equal(t, []string{"Accept-Encoding"}, second["Vary"])
	require.Equal(t, []string{"gzip"}, second["Content-Encoding"])
}

// TestNew tests that each name selects its coding.
func TestNew(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"zstd", "br", "gzip"} {
		require.Equal(t, name, New(name, 1, 1<<10).Name())
	}
}
