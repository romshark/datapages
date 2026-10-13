//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrAppUnsupportedMethod: StreamConfig belongs on a page */

func (*App) StreamConfig() (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PageParams is /params
type PageParams struct{ App *App }

func (PageParams) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageParams) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: an input a stream request does not carry */

func (PageParams) StreamConfig(
	r *http.Request, streamID datapages.StreamID,
) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PageNoRequest is /no-request
type PageNoRequest struct{ App *App }

func (PageNoRequest) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageNoRequest) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: inputs without the request */

func (PageNoRequest) StreamConfig(
	signals datapages.Signals[struct {
		Lite bool `json:"lite"`
	}],
) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PageResult is /result
type PageResult struct{ App *App }

func (PageResult) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageResult) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: another result type */

func (PageResult) StreamConfig() (datapages.SSECompressionConfig, error) {
	return datapages.SSECompressionConfig{}, nil
}

// PageNoErr is /no-err
type PageNoErr struct{ App *App }

func (PageNoErr) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageNoErr) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: no error result */

func (PageNoErr) StreamConfig() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	return body, err
}

func (PageItem) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrPathFieldNotInRoute and ErrPathMissingRouteVar: the path does not match the route */

func (PageItem) StreamConfig(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PageNoStream is /no-stream
type PageNoStream struct{ App *App }

func (PageNoStream) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrStreamConfigNoStream: the page has no stream */

func (PageNoStream) StreamConfig() (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// Shared is a stream embed.
type Shared struct{ App *App }

func (Shared) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamEmbedUnsupportedMethod: a stream embed does not configure the stream */

func (Shared) StreamConfig() (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PageShared is /shared
type PageShared struct {
	App *App
	Shared
}

func (PageShared) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}
