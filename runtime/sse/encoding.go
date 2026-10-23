package sse

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/contentcoding"
)

// Compression holds the codings a server compresses its streams with,
// in the order it prefers them. A nil Compression compresses nothing.
type Compression struct {
	codings []*contentcoding.Coding
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
		level, window := c.GzipLevel, 0
		switch name {
		case datapages.CodingZstd:
			level, window = c.ZstdLevel, c.ZstdWindow
		case datapages.CodingBrotli:
			level, window = c.BrotliQuality, c.BrotliWindow
		}
		comp.codings = append(comp.codings, contentcoding.New(string(name), level, window))
	}
	return comp
}

// CompressionKey identifies effective settings.
// Disabled compression returns "", and zero fields match their defaults.
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
		c.Codings = []datapages.Coding{
			datapages.CodingZstd, datapages.CodingBrotli, datapages.CodingGzip,
		}
	}
	c.ZstdLevel = cmp.Or(c.ZstdLevel, datapages.DefaultSSEZstdLevel)
	c.ZstdWindow = cmp.Or(c.ZstdWindow, datapages.DefaultSSEZstdWindow)
	c.BrotliQuality = cmp.Or(c.BrotliQuality, datapages.DefaultSSEBrotliQuality)
	c.BrotliWindow = cmp.Or(c.BrotliWindow, datapages.DefaultSSEBrotliWindow)
	c.GzipLevel = cmp.Or(c.GzipLevel, datapages.DefaultSSEGzipLevel)
	return c, true
}

// negotiate returns the first configured coding accepted by values,
// or nil for identity. See [contentcoding.Negotiate].
func (c *Compression) negotiate(values []string) *contentcoding.Coding {
	if c == nil {
		return nil
	}
	return contentcoding.Negotiate(c.codings, values)
}
