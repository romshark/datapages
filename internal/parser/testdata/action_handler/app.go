//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageActions is /actions
type PageActions struct{ App *App }

func (PageActions) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTWithoutSSE is /actions/without-sse
func (PageActions) POSTWithoutSSE(r *http.Request) error {
	_ = r
	return nil
}

// POSTWithSSE is /actions/with-sse
func (PageActions) POSTWithSSE(
	r *http.Request,
	sse datapages.SSE,
) error {
	_ = r
	_ = sse
	return nil
}

// PUTWithSSE is /actions/put-with-sse
func (PageActions) PUTWithSSE(
	r *http.Request,
	sse datapages.SSE,
) error {
	_ = r
	_ = sse
	return nil
}

// PATCHWithSSE is /actions/patch-with-sse
func (PageActions) PATCHWithSSE(
	r *http.Request,
	sse datapages.SSE,
) error {
	_ = r
	_ = sse
	return nil
}

// POSTSamePath is /actions
func (PageActions) POSTSamePath(r *http.Request) error {
	_ = r
	return nil
}

// PUTSamePath is /actions
func (PageActions) PUTSamePath(r *http.Request) error {
	_ = r
	return nil
}

// PATCHSamePath is /actions
func (PageActions) PATCHSamePath(r *http.Request) error {
	_ = r
	return nil
}

// DELETESamePath is /actions
func (PageActions) DELETESamePath(r *http.Request) error {
	_ = r
	return nil
}

// DELETEWithoutSSE is /actions/delete-without-sse
func (PageActions) DELETEWithoutSSE(r *http.Request) error {
	_ = r
	return nil
}

// EventFoo is "foo"
type EventFoo struct {
	Foo string `json:"foo"`
}

// Event handler WITH SSE - required for event handlers
func (PageActions) OnEventFoo(
	event EventFoo,
	sse datapages.SSE,
) error {
	_ = event
	_ = sse
	return nil
}
