package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/romshark/datapages"
)

type App struct{}

// EventFoo is "foo"
type EventFoo struct {
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

// EventBar is "bar"
type EventBar struct {
	Info string `json:"info"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// POSTSingle is /single
//
// Action with single-event dispatch.
func (PageIndex) POSTSingle(
	r *http.Request,
	dispatch datapages.Dispatch[EventFoo],
) error {
	return dispatch(EventFoo{Data: "hello"})
}

// POSTMulti is /multi
//
// Action with one dispatcher per event type.
func (PageIndex) POSTMulti(
	r *http.Request,
	dispatchFoo datapages.Dispatch[EventFoo],
	dispatchBar datapages.Dispatch[EventBar],
) error {
	return errors.Join(
		dispatchFoo(EventFoo{Data: "hello"}),
		dispatchBar(EventBar{Info: "world"}),
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
	dispatch datapages.Dispatch[EventFoo],
) error {
	_ = signals
	return dispatch(EventFoo{Data: "hello"})
}
