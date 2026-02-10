//nolint:all

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

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// PageNotFunc is /not-func
type PageNotFunc struct{ App *App }

/* ErrDispatchParamNotFunc */

func (PageNotFunc) GET(
	r *http.Request,
	dispatch int,
) (body templ.Component, err error) {
	_ = dispatch
	return body, err
}

// PageNoReturn is /no-return
type PageNoReturn struct{ App *App }

/* ErrDispatchReturnCount */

func (PageNoReturn) GET(
	r *http.Request,
	dispatch func(EventFoo),
) (body templ.Component, err error) {
	_ = dispatch
	return body, err
}

// PageWrongReturn is /wrong-return
type PageWrongReturn struct{ App *App }

/* ErrDispatchMustReturnError */

func (PageWrongReturn) GET(
	r *http.Request,
	dispatch func(EventFoo) int,
) (body templ.Component, err error) {
	_ = dispatch
	return body, err
}

// PageNoParams is /no-params
type PageNoParams struct{ App *App }

/* ErrDispatchNoParams */

func (PageNoParams) GET(
	r *http.Request,
	dispatch func() error,
) (body templ.Component, err error) {
	_ = dispatch
	return body, err
}

// PageBadEvent is /bad-event
type PageBadEvent struct{ App *App }

/* ErrDispatchParamNotEvent */

func (PageBadEvent) GET(
	r *http.Request,
	dispatch func(string) error,
) (body templ.Component, err error) {
	_ = dispatch
	return body, err
}
