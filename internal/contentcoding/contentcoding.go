// Package contentcoding pools response encoders, negotiates Accept-Encoding,
// types web fonts and decides which media types to compress.
package contentcoding

import (
	"compress/gzip"
	"io"
	"math/bits"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type encoder interface {
	io.Writer
	Flush() error
	Close() error
	Reset(w io.Writer)
}

// Coding pools the encoders of 1 content coding at 1 setting.
type Coding struct {
	name   string   // The Content-Encoding token.
	values []string // name, shared by every header it is set in.
	accept int      // Its index in the result of [accepted].
	empty  []byte   // The coding of no data.
	pool   sync.Pool
}

// Name returns the Content-Encoding token of c.
func (c *Coding) Name() string { return c.name }

// SetContentEncoding sets the Content-Encoding of h to c.
func (c *Coding) SetContentEncoding(h http.Header) { h["Content-Encoding"] = c.values }

// Empty returns a valid encoding of no data.
func (c *Coding) Empty() []byte { return c.empty }

var (
	// A single-segment frame of content size 0 and an empty last raw block.
	emptyZstd = []byte{0x28, 0xb5, 0x2f, 0xfd, 0x20, 0, 1, 0, 0}
	// WBITS 16, then a meta-block with ISLAST and ISLASTEMPTY set.
	emptyBrotli = []byte{0x06}
	emptyGzip   = []byte{
		0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 0xff, // Deflate, no flags, no time, unknown OS.
		3, 0, // A final block with only its end code.
		0, 0, 0, 0, 0, 0, 0, 0, // The CRC-32 and the size.
	}
)

// New returns the coding named name, which is zstd, br or gzip,
// at level with window, which gzip ignores.
func New(name string, level, window int) *Coding {
	switch name {
	case "zstd":
		return NewZstd(level, window)
	case "br":
		return NewBrotli(level, window)
	default:
		return NewGzip(level)
	}
}

// NewZstd returns zstd at level 1 to 22 with a window that is a power of 2
// from 1 KiB to 8 MiB.
func NewZstd(level, window int) *Coding {
	return &Coding{
		name:   "zstd",
		values: []string{"zstd"},
		accept: acceptZstd,
		empty:  emptyZstd,
		pool: sync.Pool{New: func() any {
			return newEncoder(func(w io.Writer) encoder {
				e, err := zstd.NewWriter(w,
					zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
					zstd.WithWindowSize(window),
					zstd.WithLowerEncoderMem(true),
					// Avoid the buffers used by concurrent encoding.
					zstd.WithEncoderConcurrency(1))
				if err != nil {
					panic(err) // The server options check the window.
				}
				return e
			})
		}},
	}
}

// ZstdLevel returns the lowest level that compresses like level, since
// klauspost/compress maps the levels 1 to 22 onto 4 encoder levels.
func ZstdLevel(level int) int {
	want := zstd.EncoderLevelFromZstd(level)
	for l := 1; l < level; l++ {
		if zstd.EncoderLevelFromZstd(l) == want {
			return l
		}
	}
	return level
}

// NewBrotli returns brotli at quality 1 to 11 with a window that is a power
// of 2 from 1 KiB to 16 MiB.
func NewBrotli(quality, window int) *Coding {
	return &Coding{
		name:   "br",
		values: []string{"br"},
		accept: acceptBrotli,
		empty:  emptyBrotli,
		pool: sync.Pool{New: func() any {
			return newEncoder(func(w io.Writer) encoder {
				return brotli.NewWriterOptions(w, brotli.WriterOptions{
					Quality: quality,
					LGWin:   bits.Len(uint(window)) - 1,
				})
			})
		}},
	}
}

// NewGzip returns gzip at level 1 to 9.
func NewGzip(level int) *Coding {
	return &Coding{
		name:   "gzip",
		values: []string{"gzip"},
		accept: acceptGzip,
		empty:  emptyGzip,
		pool: sync.Pool{New: func() any {
			return newEncoder(func(w io.Writer) encoder {
				e, err := gzip.NewWriterLevel(w, level)
				if err != nil {
					panic(err) // The server options check the level.
				}
				return e
			})
		}},
	}
}

// Encoder is a pooled encoder of a [Coding].
type Encoder struct {
	encoder
	sink sink
}

type sink struct{ w io.Writer }

func (s *sink) Write(p []byte) (int, error) { return s.w.Write(p) }

func newEncoder(build func(io.Writer) encoder) *Encoder {
	e := new(Encoder)
	e.encoder = build(&e.sink)
	return e
}

// Get starts a pooled encoder on w. [Coding.Put] returns it.
func (c *Coding) Get(w io.Writer) *Encoder {
	e := c.pool.Get().(*Encoder)
	e.sink.w = w
	e.Reset(&e.sink)
	return e
}

// Put returns e to the pool. The caller must not reuse e.
func (c *Coding) Put(e *Encoder) {
	e.sink.w = nil
	c.pool.Put(e)
}

// Negotiate returns the first coding accepted by the Accept-Encoding values.
// Configuration order wins over weights, and a wildcard selects only gzip.
// It ignores identity refusal as [RFC 9110 section 12.1] permits.
//
// [RFC 9110 section 12.1]: https://www.rfc-editor.org/rfc/rfc9110#section-12.1
func Negotiate(codings []*Coding, values []string) *Coding {
	if len(codings) == 0 {
		return nil
	}
	acc := accepted(values)
	for _, cd := range codings {
		if acc[cd.accept] > 0 {
			return cd
		}
	}
	if acc[acceptGzip] == 0 && acc[acceptStar] > 0 {
		for _, cd := range codings {
			if cd.accept == acceptGzip {
				return cd
			}
		}
	}
	return nil
}

const (
	acceptZstd = iota
	acceptBrotli
	acceptGzip
	acceptStar
)

// accepted returns 0 for absent, 1 for accepted and -1 for refused codings.
// The first mention wins under [RFC 9110, section 12.5.3].
//
// [RFC 9110, section 12.5.3]: https://www.rfc-editor.org/rfc/rfc9110#section-12.5.3
func accepted(values []string) (acc [4]int8) {
	for _, v := range values {
		for v != "" {
			var c string
			c, v, _ = strings.Cut(v, ",")
			name, params, _ := strings.Cut(c, ";")
			i := -1
			switch name = strings.TrimSpace(name); {
			case strings.EqualFold(name, "zstd"):
				i = acceptZstd
			case strings.EqualFold(name, "br"):
				i = acceptBrotli
			case strings.EqualFold(name, "gzip"), strings.EqualFold(name, "x-gzip"):
				i = acceptGzip
			case name == "*":
				i = acceptStar
			}
			if i < 0 || acc[i] != 0 {
				continue
			}
			acc[i] = 1
			if zeroQuality(params) {
				acc[i] = -1
			}
		}
	}
	return acc
}

func zeroQuality(params string) bool {
	for params != "" {
		var p string
		p, params, _ = strings.Cut(params, ";")
		k, v, _ := strings.Cut(p, "=")
		if strings.EqualFold(strings.TrimSpace(k), "q") {
			v = strings.TrimSpace(v)
			return v == "0" ||
				strings.HasPrefix(v, "0.") &&
					strings.Trim(v[2:], "0") == ""
		}
	}
	return false
}

// Compressible reports whether compression usually shrinks
// the media type of contentType. The SSE package handles event streams.
func Compressible(contentType string) bool {
	mt, _, _ := strings.Cut(contentType, ";")
	mt = strings.ToLower(strings.TrimSpace(mt))
	switch {
	case mt == "text/event-stream":
		return false
	case strings.HasPrefix(mt, "text/"),
		strings.HasSuffix(mt, "+json"), strings.HasSuffix(mt, "+xml"):
		return true
	}
	switch mt {
	case "application/json", "application/javascript", "application/xml",
		"application/wasm",
		// Text that the shared-mime-info database of Linux types as application.
		"application/yaml", "application/x-yaml", "application/toml",
		// TrueType and OpenType fonts, which WOFF and WOFF2 hold compressed.
		"font/ttf", "font/otf", "font/collection", "font/sfnt",
		// Icons and bitmaps, which mostly hold uncompressed pixels.
		"image/bmp", "image/x-icon", "image/vnd.microsoft.icon":
		return true
	}
	return false
}

// fontTypes overrides the host's mime table, which can type .otf and .otc as
// OpenDocument templates. shared-mime-info 2.5.1 types both that way.
var fontTypes = map[string]string{
	".otf":   "font/otf",
	".ttf":   "font/ttf",
	".ttc":   "font/collection",
	".otc":   "font/collection",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

// FontType returns the font type of ext in any case, or "" for other files.
func FontType(ext string) string {
	return fontTypes[strings.ToLower(ext)]
}

// TypeByExtension returns [FontType], else [mime.TypeByExtension].
func TypeByExtension(ext string) string {
	if t := FontType(ext); t != "" {
		return t
	}
	return mime.TypeByExtension(ext)
}

// NoTransform reports whether h has Cache-Control: no-transform.
func NoTransform(h http.Header) bool {
	for _, v := range h.Values("Cache-Control") {
		for t := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), "no-transform") {
				return true
			}
		}
	}
	return false
}

// varyAcceptEncoding is shared by every header it is set in.
var varyAcceptEncoding = []string{"Accept-Encoding"}

// AddVaryAcceptEncoding adds Accept-Encoding to the Vary header of h unless it
// lists Accept-Encoding or *.
func AddVaryAcceptEncoding(h http.Header) {
	vary := h["Vary"]
	if vary == nil {
		h["Vary"] = varyAcceptEncoding
		return
	}
	for _, v := range vary {
		for t := range strings.SplitSeq(v, ",") {
			t = strings.TrimSpace(t)
			if t == "*" || strings.EqualFold(t, "Accept-Encoding") {
				return
			}
		}
	}
	h["Vary"] = append(vary, "Accept-Encoding")
}
