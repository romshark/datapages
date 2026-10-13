package sse

import (
	"cmp"
	"compress/gzip"
	"fmt"
	"io"
	"math/bits"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/romshark/datapages"
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

// Compression holds the codings a server compresses its streams with,
// in the order it prefers them. A nil Compression compresses nothing.
type Compression struct {
	codings []*coding
}

// NewCompression returns the compression conf configures, nil when conf disables it.
// A nil conf selects the defaults of [datapages.SSECompressionConfig].
// conf must have passed [datapages.SSECompressionConfig.Validate].
func NewCompression(conf *datapages.SSECompressionConfig) *Compression {
	c, ok := resolve(conf)
	if !ok {
		return nil
	}
	comp := new(Compression)
	for _, name := range c.Codings {
		switch name {
		case datapages.SSECodingZstd:
			comp.codings = append(comp.codings, newZstdCoding(c.ZstdLevel, c.ZstdWindow))
		case datapages.SSECodingBrotli:
			comp.codings = append(comp.codings,
				newBrotliCoding(c.BrotliQuality, c.BrotliWindow))
		case datapages.SSECodingGzip:
			comp.codings = append(comp.codings, newGzipCoding(c.GzipLevel))
		}
	}
	return comp
}

// CompressionKey returns what tells apart how configurations compress,
// "" for one that disables compression. A zero field and the default
// it selects give the same key.
func CompressionKey(conf *datapages.SSECompressionConfig) string {
	c, ok := resolve(conf)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, name := range c.Codings {
		b.WriteString(string(name))
		b.WriteByte(',')
	}
	fmt.Fprintf(&b, "%d,%d,%d,%d,%d",
		c.ZstdLevel, c.ZstdWindow, c.BrotliQuality, c.BrotliWindow, c.GzipLevel)
	return b.String()
}

// resolve returns conf with the defaults in place of its zero fields,
// and false when conf disables compression.
func resolve(
	conf *datapages.SSECompressionConfig,
) (datapages.SSECompressionConfig, bool) {
	var c datapages.SSECompressionConfig
	if conf != nil {
		c = *conf
	}
	if c.Disabled {
		return c, false
	}
	if len(c.Codings) == 0 {
		c.Codings = []datapages.SSECoding{
			datapages.SSECodingZstd, datapages.SSECodingBrotli, datapages.SSECodingGzip,
		}
	}
	c.ZstdLevel = cmp.Or(c.ZstdLevel, datapages.DefaultSSEZstdLevel)
	c.ZstdWindow = cmp.Or(c.ZstdWindow, datapages.DefaultSSEZstdWindow)
	c.BrotliQuality = cmp.Or(c.BrotliQuality, datapages.DefaultSSEBrotliQuality)
	c.BrotliWindow = cmp.Or(c.BrotliWindow, datapages.DefaultSSEBrotliWindow)
	c.GzipLevel = cmp.Or(c.GzipLevel, datapages.DefaultSSEGzipLevel)
	return c, true
}

// coding pools encoders. A stream takes one only when it sends its first event.
type coding struct {
	name   string // The Content-Encoding token.
	accept int    // Its index in the result of [accepted].
	empty  []byte // The coding of no data, for a stream without events.
	pool   sync.Pool
}

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

func newZstdCoding(level, window int) *coding {
	return &coding{
		name: "zstd", accept: acceptZstd, empty: emptyZstd,
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				e, err := zstd.NewWriter(w,
					zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
					zstd.WithWindowSize(window),
					zstd.WithLowerEncoderMem(true),
					// Avoid the buffers used by concurrent encoding.
					zstd.WithEncoderConcurrency(1))
				if err != nil {
					panic(err) // WithSSECompression checks the window.
				}
				return e
			})
		}},
	}
}

func newBrotliCoding(quality, window int) *coding {
	return &coding{
		name: "br", accept: acceptBrotli, empty: emptyBrotli,
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				return brotli.NewWriterOptions(w, brotli.WriterOptions{
					Quality: quality,
					LGWin:   bits.Len(uint(window)) - 1,
				})
			})
		}},
	}
}

func newGzipCoding(level int) *coding {
	return &coding{
		name: "gzip", accept: acceptGzip, empty: emptyGzip,
		pool: sync.Pool{New: func() any {
			return newPooledEncoder(func(w io.Writer) encoder {
				e, err := gzip.NewWriterLevel(w, level)
				if err != nil {
					panic(err) // WithSSECompression checks the level.
				}
				return e
			})
		}},
	}
}

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

// negotiate returns the first coding of c that the Accept-Encoding
// values accept, nil for none. A wildcard enables gzip only.
func (c *Compression) negotiate(values []string) *coding {
	if c == nil {
		return nil
	}
	acc := accepted(values)
	for _, cd := range c.codings {
		if acc[cd.accept] > 0 {
			return cd
		}
	}
	if acc[acceptGzip] == 0 && acc[acceptStar] > 0 {
		for _, cd := range c.codings {
			if cd.accept == acceptGzip {
				return cd
			}
		}
	}
	return nil
}

// The indexes of the result of [accepted].
const (
	acceptZstd = iota
	acceptBrotli
	acceptGzip
	acceptStar
)

// accepted reads the values of an Accept-Encoding header,
// see [RFC 9110, section 12.5.3]. For each of zstd, br, gzip and *,
// it returns 0 when the values do not mention it, 1 when they accept it
// and -1 when they refuse it. The first mention counts.
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
