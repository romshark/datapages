//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageNotStruct is /not-struct/{id}
type PageNotStruct struct{ App *App }

/* ErrPathParamNotStruct */

func (PageNotStruct) GET(r *http.Request, path int) (body templ.Component, err error) {
	_ = path
	return body, err
}

// PageUnexported is /unexported/{id}
type PageUnexported struct{ App *App }

/* ErrPathFieldUnexported */

func (PageUnexported) GET(
	r *http.Request,
	path struct {
		id string `path:"id"`
	},
) (body templ.Component, err error) {
	_ = path
	return body, err
}

// PageNotString is /not-string/{id}
type PageNotString struct{ App *App }

/* ErrPathFieldNotString */

func (PageNotString) GET(
	r *http.Request,
	path struct {
		ID int `path:"id"`
	},
) (body templ.Component, err error) {
	_ = path
	return body, err
}

// PageMissingTag is /missing-tag/{id}
type PageMissingTag struct{ App *App }

/* ErrPathFieldMissingTag */

func (PageMissingTag) GET(
	r *http.Request,
	path struct {
		ID string
	},
) (body templ.Component, err error) {
	_ = path
	return body, err
}

// PageNotInRoute is /not-in-route
type PageNotInRoute struct{ App *App }

/* ErrPathFieldNotInRoute */

func (PageNotInRoute) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
) (body templ.Component, err error) {
	_ = path
	return body, err
}

// PageMissingVar is /missing-var/{id}
type PageMissingVar struct{ App *App }

/* ErrPathMissingRouteVar */

func (PageMissingVar) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}
