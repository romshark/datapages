//nolint:all
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

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// EventFoo is "foo"
type EventFoo struct {
	Foo string `json:"foo"`
}

/* ErrEventMissingComm */

type EventNoComment struct {
	X int `json:"x"`
}

/* ErrEventSubjectInvalid */

// EventBadSubject0 is ""
type EventBadSubject0 struct {
	Z int `json:"z"`
}

// PageEventTest is /event-test
type PageEventTest struct{ App *App }

func (PageEventTest) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

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

/* ErrEventFieldUnexported */

// EventUnexported is "unexported"
type EventUnexported struct {
	unexported string `json:"u"`
}

/* ErrEventFieldMissingTag */

// EventMissingTag is "missing_tag"
type EventMissingTag struct {
	Field string
}

/* ErrEventFieldUnexported */

// EventNested is "bad"
//
// Even if "Nested" itself is exported and tagged, the type it refers to has issues.
// But our validator recurses.
type EventNested struct {
	Nested EventUnexported `json:"n"`
}

/* ErrEventInvalidComm */

// EventInvalidComm handles "abc"
type EventInvalidComm struct {
	X int `json:"x"`
}

/* ErrEventInvalidComm */

// NotTheRightTypeName is "abc"
type EventRightName struct {
	X int `json:"x"`
}

/* ErrEventSubjectInvalid */

// EventBadSubject is ""
type EventBadSubject struct {
	X int `json:"x"`
}

/* ErrEventSubjectInvalid */

// EventBadSubject2 is ""
type EventBadSubject2 struct {
	X int `json:"x"`
}
