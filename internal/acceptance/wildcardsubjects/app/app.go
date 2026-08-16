// Package app exercises an event whose subject field has no signal to fill it in,
// so a page subscribes to every value of that subject.
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// EventNoted is "noted"
//
// The subject field carries no signal tag.
// Every stream of the page therefore wants every value of it.
type EventNoted struct {
	SubjectTopic string

	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

func (PageIndex) OnNoted(
	event EventNoted,
	sse datapages.SSE,
) error {
	return sse.PatchElement(
		templ.Raw(`<div id="noted">` + event.Text + `</div>`),
	)
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
