package sse

import (
	"compress/gzip"
	"io"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// encoder is the common API of the supported compressors.
type encoder interface {
	io.Writer
	// Flush makes all written bytes available to the client.
	Flush() error
	// Close finishes the compressed stream.
	Close() error
	// Reset reuses the encoder with w.
	Reset(w io.Writer)
}

// coding pools encoders. A stream takes one only when it sends its first event.
type coding struct {
	name  string // The Content-Encoding token.
	empty []byte // The coding of no data, for a stream without events.
	pool  sync.Pool
}

// A 1 MiB window covers repeated fragment renders. For 20 100 KB renders with
// one changed item, zstd, brotli and gzip output 0.9%, 0.7% and 12.3% of the input.
// zstd takes 13% to 20% of gzip's time. At these levels, zstd retains
// about 1.5 MB per stream and brotli retains 3 to 4 MB.
var (
	zstdCoding = coding{
		name: "zstd",
		// A single-segment frame of content size 0 and an empty last raw block.
		empty: []byte{0x28, 0xb5, 0x2f, 0xfd, 0x20, 0, 1, 0, 0},
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				e, err := zstd.NewWriter(w,
					zstd.WithEncoderLevel(zstd.SpeedFastest),
					zstd.WithWindowSize(1<<20),
					zstd.WithLowerEncoderMem(true),
					// Avoid the buffers used by concurrent encoding.
					zstd.WithEncoderConcurrency(1))
				if err != nil {
					panic(err) // The options are constant.
				}
				return e
			})
		}},
	}
	brotliCoding = coding{
		name: "br",
		// WBITS 16, then a meta-block with ISLAST and ISLASTEMPTY set.
		empty: []byte{0x06},
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				return brotli.NewWriterOptions(w, brotli.WriterOptions{
					Quality: 4, LGWin: 20,
				})
			})
		}},
	}
	gzipCoding = coding{
		name: "gzip",
		empty: []byte{
			0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 0xff, // Deflate, no flags, no time, unknown OS.
			3, 0, // A final block with only its end code.
			0, 0, 0, 0, 0, 0, 0, 0, // The CRC-32 and the size.
		},
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				return gzip.NewWriter(w)
			})
		}},
	}
)

// pooledEncoder clears sink to release the response without another reset.
type pooledEncoder struct {
	encoder
	sink sink
}

type sink struct{ w io.Writer }

func (s *sink) Write(p []byte) (int, error) { return s.w.Write(p) }

func newPooledEncoder(newEncoder func(io.Writer) encoder) *pooledEncoder {
	e := new(pooledEncoder)
	e.encoder = newEncoder(&e.sink)
	return e
}

// take starts a pooled encoder on w.
func (c *coding) take(w io.Writer) *pooledEncoder {
	e := c.pool.Get().(*pooledEncoder)
	e.sink.w = w
	e.Reset(&e.sink)
	return e
}

func (c *coding) put(e *pooledEncoder) {
	e.sink.w = nil
	c.pool.Put(e)
}

// negotiate returns the first accepted coding in this order: zstd, br, gzip.
// A wildcard enables gzip only. See [RFC 9110, section 12.5.3].
//
// [RFC 9110, section 12.5.3]: https://www.rfc-editor.org/rfc/rfc9110#section-12.5.3
func negotiate(values []string) *coding {
	const (
		iZstd = iota
		iBrotli
		iGzip
		iStar
	)
	// 0 means absent, 1 accepted and -1 refused. The first mention wins.
	var acc [4]int8
	for _, v := range values {
		for v != "" {
			var c string
			c, v, _ = strings.Cut(v, ",")
			name, params, _ := strings.Cut(c, ";")
			i := -1
			switch name = strings.TrimSpace(name); {
			case strings.EqualFold(name, "zstd"):
				i = iZstd
			case strings.EqualFold(name, "br"):
				i = iBrotli
			case strings.EqualFold(name, "gzip"), strings.EqualFold(name, "x-gzip"):
				i = iGzip
			case name == "*":
				i = iStar
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
	switch {
	case acc[iZstd] > 0:
		return &zstdCoding
	case acc[iBrotli] > 0:
		return &brotliCoding
	case acc[iGzip] > 0, acc[iGzip] == 0 && acc[iStar] > 0:
		return &gzipCoding
	}
	return nil
}

// zeroQuality reports whether q explicitly refuses the coding.
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
