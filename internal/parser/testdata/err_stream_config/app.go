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

func (*App) StreamConfig() datapages.StreamConfig {
	return datapages.StreamConfig{}
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

func (PageResult) StreamConfig() datapages.SSECompressionConfig {
	return datapages.SSECompressionConfig{}
}

// PageNoErr is /no-err
type PageNoErr struct{ App *App }

func (PageNoErr) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageNoErr) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: request-scoped method without error */

func (PageNoErr) StreamConfig(r *http.Request) datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// PageStaticErr is /static-err
type PageStaticErr struct{ App *App }

func (PageStaticErr) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

func (PageStaticErr) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamConfigSignature: static method with error */

func (PageStaticErr) StreamConfig() (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
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

func (PageNoStream) StreamConfig() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// Shared is a stream embed.
type Shared struct{ App *App }

func (Shared) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamEmbedUnsupportedMethod: a stream embed does not configure the stream */

func (Shared) StreamConfig() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// PageShared is /shared
type PageShared struct {
	App *App
	Shared
}

func (PageShared) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}
