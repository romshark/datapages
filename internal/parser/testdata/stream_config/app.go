//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{ window int }

// PageIndex is /
//
// Its stream compresses with the window App holds.
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageIndex) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

func (p PageIndex) StreamConfig() datapages.StreamConfig {
	return datapages.StreamConfig{
		Compression: &datapages.SSECompressionConfig{ZstdWindow: p.App.window},
	}
}

// PagePlain is /plain
//
// Its stream keeps the compression of the server.
type PagePlain struct{ App *App }

func (PagePlain) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PagePlain) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

// PageAdaptive is /adaptive
//
// Its stream compresses as each stream request asks.
type PageAdaptive struct{ App *App }

func (PageAdaptive) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageAdaptive) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

func (PageAdaptive) StreamConfig(
	signals datapages.Signals[struct {
		Lite bool `json:"lite"`
	}],
	r *http.Request,
) (datapages.StreamConfig, error) {
	if !signals.Values.Lite && r.Header.Get("Save-Data") != "on" {
		return datapages.StreamConfig{}, nil
	}
	return datapages.StreamConfig{Compression: &datapages.SSECompressionConfig{
		Codings: []datapages.Coding{datapages.CodingGzip},
	}}, nil
}

// PageBoard is /board/{name}
//
// Its stream compresses by the board it shows.
type PageBoard struct{ App *App }

func (PageBoard) GET(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (body datapages.Component, err error) {
	return body, err
}

func (PageBoard) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

func (PageBoard) StreamConfig(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (datapages.StreamConfig, error) {
	if path.Values.Name != "big" {
		return datapages.StreamConfig{}, nil
	}
	return datapages.StreamConfig{
		Compression: &datapages.SSECompressionConfig{ZstdWindow: 4 << 20},
	}, nil
}
