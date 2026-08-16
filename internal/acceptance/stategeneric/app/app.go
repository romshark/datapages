// Package app exercises per-tab state reached through abstract pages: a
// generic abstract page instantiated on several state types, a chain of two
// generic abstract pages, and one embedded by pointer.
//
// The interesting property is isolation. Two tabs of one page, and two pages
// bound to one state type, must each get their own value, and a handler must
// be given the value of the tab whose request it is answering.
package app

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	// allocations counts how many state values were opened.
	// It shows whether two tabs got one value or two.
	allocations atomic.Int64
}

// EventPing is "ping"
type EventPing struct {
	SubjectStateID string
}

// StateCounter is the state of PageCount, reached through the generic base.
type StateCounter struct {
	N int
}

// StateLabel is the state of PageLabel,
// reached through the same generic base with a different type argument.
type StateLabel struct {
	Text string
}

// StateNested is the state of PageNested, reached through two levels of
// generic abstract page.
type StateNested struct {
	N int
}

// StatePointer is the state of PagePointer, reached through an embed by
// pointer.
type StatePointer struct {
	N int
}

// Base is a generic abstract page.
// Every page that embeds Base[T] gets these handlers with state *T.
type Base[S any] struct{ App *App }

func (b Base[S]) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *S,
) error {
	_, _, _ = r, streamID, state
	b.App.allocations.Add(1)
	return nil
}

func (b Base[S]) OnPing(
	event EventPing,
	sse datapages.SSE,
	state *S,
) error {
	_ = event
	return sse.PatchElement(templ.Raw(
		fmt.Sprintf(`<div id="state">%+v</div>`, *state),
	))
}

// PageCount is /count
type PageCount struct {
	App *App
	Base[StateCounter]
}

func (PageCount) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">count</pre>`), nil
}

// POSTBump is /count/bump
func (PageCount) POSTBump(
	_ *http.Request,
	state *StateCounter,
	stateID string,
	dispatch func(EventPing) error,
) error {
	state.N++
	return dispatch(EventPing{SubjectStateID: stateID})
}

// PageLabel is /label
type PageLabel struct {
	App *App
	Base[StateLabel]
}

func (PageLabel) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">label</pre>`), nil
}

// POSTSet is /label/set
func (PageLabel) POSTSet(
	_ *http.Request,
	state *StateLabel,
	stateID string,
	signals struct {
		Text string `json:"text"`
	},
	dispatch func(EventPing) error,
) error {
	state.Text = signals.Text
	return dispatch(EventPing{SubjectStateID: stateID})
}

// PageEmbedOnly is /embed-only
//
// A page that declares no handler of its own. Its stream, its event handler and
// the state type they are bound to all arrive through the embed.
type PageEmbedOnly struct {
	App *App
	Base[StateLabel]
}

func (PageEmbedOnly) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">embed only</pre>`), nil
}

// Mid is a generic abstract page that embeds another one, passing its own
// type parameter down.
type Mid[S any] struct {
	App *App
	Base[S]
}

// PageNested is /nested
//
// Its stream, its event handler and its state type arrive through two levels
// of generic abstract page.
type PageNested struct {
	App *App
	Mid[StateNested]
}

func (PageNested) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">nested</pre>`), nil
}

// POSTBump is /nested/bump
func (PageNested) POSTBump(
	_ *http.Request,
	state *StateNested,
	stateID string,
	dispatch func(EventPing) error,
) error {
	state.N++
	return dispatch(EventPing{SubjectStateID: stateID})
}

// PagePointer is /pointer
//
// Embeds the abstract page by pointer, which Go allows and which names the
// same type.
type PagePointer struct {
	App *App
	*Base[StatePointer]
}

func (PagePointer) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">pointer</pre>`), nil
}

// POSTBump is /pointer/bump
func (PagePointer) POSTBump(
	_ *http.Request,
	state *StatePointer,
	stateID string,
	dispatch func(EventPing) error,
) error {
	state.N++
	return dispatch(EventPing{SubjectStateID: stateID})
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(fmt.Sprintf(`<pre id="echo">allocations=%d</pre>`,
		p.App.allocations.Load())), nil
}
