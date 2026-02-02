//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// GET handler with SSE parameter (optional for GET)
func (PageIndex) HEAD(r *http.Request, sse *datastar.ServerSentEventGenerator) (head templ.Component, err error) {
	_ = sse
	return head, err
}

// PageActions is /actions
type PageActions struct{ App *App }

func (PageActions) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// POSTWithoutSse is /actions/without-sse
func (PageActions) POSTWithoutSse(r *http.Request) error {
	_ = r
	return nil
}

// POSTWithSse is /actions/with-sse
func (PageActions) POSTWithSse(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
) error {
	_ = r
	_ = sse
	return nil
}

// PUTWithSse is /actions/put-with-sse
func (PageActions) PUTWithSse(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
) error {
	_ = r
	_ = sse
	return nil
}

// DELETEWithoutSse is /actions/delete-without-sse
func (PageActions) DELETEWithoutSse(r *http.Request) error {
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
	sse *datastar.ServerSentEventGenerator,
) error {
	_ = event
	_ = sse
	return nil
}
