// Package app reproduces a delivery gap between the generated subscriptions
// and the message broker this repository ships for single-process use.
//
// A subject field without a signal tag is not scoped by anything the stream
// chose. The generated subscription therefore asks for every value of it and
// spells that as the NATS wildcard "*". The publisher publishes the concrete
// subject. modules/msgbroker/inmem matches subjects as map keys and implements
// no wildcard matching. The two never meet and the event is dropped.
//
// Nothing reports it: the dispatch succeeds, the action returns 200, and the
// handler simply never runs. The same application on a NATS broker works.
//
// See options.json. The case is expected to fail until the in-memory broker
// matches wildcards or the generator stops emitting them for it.
package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventNoted is "noted"
//
// The subject field carries no signal tag. Every stream of the page therefore
// wants every value of it.
type EventNoted struct {
	SubjectTopic string

	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw("index"), nil
}

func (PageIndex) OnNoted(
	event EventNoted,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(`<div id="noted">` + event.Text + `</div>`))
}

// POSTNote is /note
func (PageIndex) POSTNote(
	_ *http.Request,
	signals struct {
		Topic string `json:"topic"`
		Text  string `json:"text"`
	},
	dispatch func(EventNoted) error,
) error {
	return dispatch(EventNoted{SubjectTopic: signals.Topic, Text: signals.Text})
}
