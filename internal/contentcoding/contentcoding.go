// Package contentcoding pools response encoders and negotiates Accept-Encoding.
package contentcoding

import (
	"compress/gzip"
	"io"
	"math/bits"
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
	name   string // The Content-Encoding token.
	accept int    // Its index in the result of [accepted].
	empty  []byte // The coding of no data.
	pool   sync.Pool
}

// Name returns the Content-Encoding token of c.
func (c *Coding) Name() string { return c.name }

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

// NewZstd returns zstd at level 1 to 22 with a window that is a power of 2
// from 1 KiB to 8 MiB.
func NewZstd(level, window int) *Coding {
	return &Coding{
		name: "zstd", accept: acceptZstd, empty: emptyZstd,
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

// NewBrotli returns brotli at quality 1 to 11 with a window that is a power
// of 2 from 1 KiB to 16 MiB.
func NewBrotli(quality, window int) *Coding {
	return &Coding{
		name: "br", accept: acceptBrotli, empty: emptyBrotli,
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
		name: "gzip", accept: acceptGzip, empty: emptyGzip,
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
