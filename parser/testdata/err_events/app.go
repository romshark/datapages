package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) { return body, err }

// EventFoo is "foo"
type EventFoo struct {
	Foo string `json:"foo"`
}

/* ErrEventMissingComm */

type EventNoComment struct {
	X int `json:"x"`
}

/* ErrEventInvalidComm */

// EventBadComment is foo
type EventBadComment struct {
	Y int `json:"y"`
}

// PageEventTest is /event-test
type PageEventTest struct{ App *App }

func (PageEventTest) GET(r *http.Request) (body templ.Component, err error) { return body, err }

/* ErrEvHandFirstArgNotEvent */

func (PageEventTest) OnFirstArgNotNamed(
	notEvent EventFoo,
) error {
	return nil
}

/* ErrEvHandFirstArgTypeNotEvent */

func (PageEventTest) OnFirstArgWrongType(
	event int,
) error {
	return nil
}

func (PageEventTest) OnFirstDuplicate(
	event EventFoo,
) error {
	return nil
}

/* ErrEvHandDuplicate */

func (PageEventTest) OnSecondDuplicate(
	event EventFoo,
) error {
	return nil
}
