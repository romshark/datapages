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

// PageLegacyFunc is /legacy-func
type PageLegacyFunc struct{ App *App }

/* ErrDispatchParamLegacy */

func (PageLegacyFunc) GET(
	r *http.Request,
	dispatch func(EventFoo) error,
) (body datapages.Component, err error) {
	_ = dispatch
	return body, err
}

// PageLegacyName is /legacy-name
type PageLegacyName struct{ App *App }

/* ErrDispatchParamLegacy */

func (PageLegacyName) GET(
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
