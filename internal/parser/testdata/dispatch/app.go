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
	dispatch datapages.Dispatcher[EventFoo],
) error {
	return dispatch.Dispatch(EventFoo{Data: "hello"})
}

// POSTMulti is /multi
//
// Action with one dispatcher per event type.
func (PageIndex) POSTMulti(
	r *http.Request,
	dispatchFoo datapages.Dispatcher[EventFoo],
	dispatchBar datapages.Dispatcher[EventBar],
) error {
	return errors.Join(
		dispatchFoo.Dispatch(EventFoo{Data: "hello"}),
		dispatchBar.Dispatch(EventBar{Info: "world"}),
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
	dispatch datapages.Dispatcher[EventFoo],
) error {
	_ = signals
	return dispatch.Dispatch(EventFoo{Data: "hello"})
}
