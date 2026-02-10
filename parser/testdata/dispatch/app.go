package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// EventFoo is "foo"
type EventFoo struct {
	Data string `json:"data"`
}

// EventBar is "bar"
type EventBar struct {
	Info string `json:"info"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// POSTSingle is /single
//
// Action with single-event dispatch.
func (PageIndex) POSTSingle(
	r *http.Request,
	dispatch func(EventFoo) error,
) error {
	return dispatch(EventFoo{Data: "hello"})
}

// POSTMulti is /multi
//
// Action with multi-event dispatch.
func (PageIndex) POSTMulti(
	r *http.Request,
	dispatch func(EventFoo, EventBar) error,
) error {
	return dispatch(
		EventFoo{Data: "hello"},
		EventBar{Info: "world"},
	)
}

// POSTWithSignals is /with-signals
//
// Action with signals before dispatch.
func (PageIndex) POSTWithSignals(
	r *http.Request,
	signals struct {
		Name string `json:"name"`
	},
	dispatch func(EventFoo) error,
) error {
	_ = signals
	return dispatch(EventFoo{Data: "hello"})
}
