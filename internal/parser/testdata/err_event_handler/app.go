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

// EventFoo is "foo"
type EventFoo struct {
	Foo string `json:"foo"`
}

// EventBar is "bar"
type EventBar struct {
	Bar string `json:"bar"`
}

// EventBazz is "bazz"
type EventBazz struct {
	Bazz string `json:"bazz"`
}

// EventFuzz is "fuzz"
type EventFuzz struct {
	Fuzz string `json:"fuzz"`
}

// EventQux is "qux"
type EventQux struct {
	Qux string `json:"qux"`
}

// EventQuux is "quux"
type EventQuux struct {
	Quux string `json:"quux"`
}

// EventCorge is "corge"
type EventCorge struct {
	Corge string `json:"corge"`
}

/* ErrEvHandSecondArgNotSSE */

func (PageIndex) OnQux(
	event EventQux,
) error {
	_ = event
	return nil
}

/* ErrSignatureUnsupportedInput */

func (PageIndex) OnCorge(
	event EventCorge,
	sse datapages.SSE,
	unknownParam string,
) error {
	_ = event
	_ = sse
	_ = unknownParam
	return nil
}

/* ErrEvHandSecondArgNotSSE */

func (PageIndex) OnQuux(
	event EventQuux,
	notSSE int,
) error {
	_ = event
	_ = notSSE
	return nil
}

func (PageIndex) OnFoo(
	event EventFoo,
	sse datapages.SSE,
) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
}

func (PageIndex) OnBar(
	event EventBar,
	sse datapages.SSE,
) (int, error) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return 0, nil
}

func (PageIndex) OnBazz(
	event EventBazz,
	sse datapages.SSE,
) (
	error, error,
) { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return nil, nil
}

func (PageIndex) OnFuzz(
	event EventFuzz,
	sse datapages.SSE,
) int { /* ErrEvHandReturnMustBeError */
	_ = event
	_ = sse
	return 0
}
