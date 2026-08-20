//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventFoo is "foo"
type EventFoo struct {
	Data string `json:"data"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// PageFuncParam is /func-param
type PageFuncParam struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageFuncParam) GET(
	r *http.Request,
	dispatch func(EventFoo) error,
) (body datapages.Component, err error) {
	_ = dispatch
	return body, err
}

// PageDispatchInt is /dispatch-int
type PageDispatchInt struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageDispatchInt) GET(
	r *http.Request,
	dispatch int,
) (body datapages.Component, err error) {
	_ = dispatch
	return body, err
}

// PageBadEvent is /bad-event
type PageBadEvent struct{ App *App }

/* ErrDispatchParamNotEvent */

func (PageBadEvent) GET(
	r *http.Request,
	dispatchString datapages.Dispatcher[string],
) (body datapages.Component, err error) {
	_ = dispatchString
	return body, err
}

// PageDuplicate is /duplicate
type PageDuplicate struct{ App *App }

/* ErrDispatchDuplicate */

func (PageDuplicate) GET(
	r *http.Request,
	dispatchFoo datapages.Dispatcher[EventFoo],
	dispatchFooAgain datapages.Dispatcher[EventFoo],
) (body datapages.Component, err error) {
	_, _ = dispatchFoo, dispatchFooAgain
	return body, err
}
