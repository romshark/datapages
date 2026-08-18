// Package app exercises one state type shared by two pages,
// and an app-level action that reaches it.
//
// An abstract page gives its state type to every page that embeds it.
// The pages are still separate and their tabs must not share a value.
// An app-level action must act on the tab that called it,
// or refuse when the calling tab is bound to no page that uses the type.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// EventChanged is "changed"
type EventChanged struct {
	SubjectStateID datapages.SubjectStateID
}

// TabContext is the state of every page that embeds Base.
type TabContext struct {
	Counter int
	Note    string
}

// Base is an abstract page. Embedding it binds a page to TabContext.
type Base struct{ App *App }

func (Base) StreamOpen(
	r *http.Request,
	streamID uint64,
	state *TabContext,
) error {
	_, _, _ = r, streamID, state
	return nil
}

func (Base) OnChanged(
	event EventChanged,
	sse datapages.SSE,
	state *TabContext,
) error {
	_ = event
	return sse.PatchElement(templ.Raw(
		fmt.Sprintf(`<div id="state">%+v</div>`, *state),
	))
}

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

// POSTNote is /note
func (PageIndex) POSTNote(
	_ *http.Request,
	state *TabContext,
	stateID string,
	signals struct {
		Note string `json:"note"`
	},
	dispatch datapages.Dispatch[EventChanged],
) error {
	state.Note = signals.Note
	return dispatch(EventChanged{
		SubjectStateID: datapages.SubjectStateID(stateID),
	})
}

// PageOther is /other
type PageOther struct {
	App *App
	Base
}

func (PageOther) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">other</pre>`), nil
}

// PagePlain is /plain
//
// A page with no state at all. A tab of it can still call the app action,
// which is where the runtime has to say no.
type PagePlain struct{ App *App }

func (PagePlain) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">plain</pre>`), nil
}

// POSTBump is /bump
//
// An app-level action bound to the shared state type.
// It works from a tab of any page that uses TabContext.
func (a *App) POSTBump(
	_ *http.Request,
	state *TabContext,
	stateID string,
	dispatch datapages.Dispatch[EventChanged],
) error {
	state.Counter++
	return dispatch(EventChanged{
		SubjectStateID: datapages.SubjectStateID(stateID),
	})
}
