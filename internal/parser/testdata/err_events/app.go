//nolint:all
package app

import (
	"net/http"

	"datapagestest/fixture/err_events/subpkg"

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

func (PageEventTest) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrSignatureEvHandMissingEvent */
/* ErrSignatureEvHandMissingSSE */
/* ErrSignatureUnsupportedInput */

func (PageEventTest) OnArgWrongType(
	event int,
) error {
	return nil
}

/* ErrSignatureEvHandMissingSSE */

func (PageEventTest) OnFirstDuplicate(
	event EventFoo,
) error {
	return nil
}

/* ErrEvHandDuplicate */
/* ErrSignatureEvHandMissingSSE */

func (PageEventTest) OnSecondDuplicate(
	event EventFoo,
) error {
	return nil
}

// EventAlpha is "alpha"
type EventAlpha struct {
	A string `json:"a"`
}

// EventBeta is "beta"
type EventBeta struct {
	B string `json:"b"`
}

// PageMultiEvent is /multi-event
type PageMultiEvent struct{ App *App }

func (PageMultiEvent) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrSignatureEvHandMultipleEvents */

func (PageMultiEvent) OnBoth(
	alpha EventAlpha,
	beta EventBeta,
	sse datapages.SSE,
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

/* ErrEventFieldDuplicateTag */

// EventDuplicateTag is "duplicate_tag"
type EventDuplicateTag struct {
	A string `json:"x"`
	B string `json:"x"`
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

/* ErrEventFieldMissingTag */

// EventEmptyJSONTag is "empty_json_tag"
//
// json:"" has an empty name — encoding/json binds nothing, so it should be rejected.
type EventEmptyJSONTag struct {
	Bad string `json:""`
}

// EventJSONExcluded is "json_excluded"
//
// Fields with json:"-" are intentionally omitted; no errors should be reported.
type EventJSONExcluded struct {
	Keep    string `json:"keep"`
	ignored int    `json:"-"` //nolint:all
}

/* ErrEventFieldUnexported */

// EventSameModuleSubpkg is "same_module_subpkg"
//
// Validator must still recurse into same-module sub-package types.
type EventSameModuleSubpkg struct {
	Bad subpkg.BadFields `json:"bad"`
}

/* ErrEventSubjectInvalid: a NATS wildcard in the declared subject.
   The generator concatenates it into the subscription, where only the server
   rejects it, by closing the connection the whole process shares. */

// EventTailWildcard is "noted.>"
type EventTailWildcard struct {
	Text string `json:"text"`
}
