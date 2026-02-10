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

// PageNotStruct is /not-struct
type PageNotStruct struct{ App *App }

/* ErrQueryParamNotStruct */

func (PageNotStruct) GET(r *http.Request, query int) (body templ.Component, err error) {
	_ = query
	return body, err
}

// PageUnexported is /unexported
type PageUnexported struct{ App *App }

/* ErrQueryFieldUnexported */

func (PageUnexported) GET(
	r *http.Request,
	query struct {
		term string `query:"t"`
	},
) (body templ.Component, err error) {
	_ = query
	return body, err
}

// PageMissingTag is /missing-tag
type PageMissingTag struct{ App *App }

/* ErrQueryFieldMissingTag */

func (PageMissingTag) GET(
	r *http.Request,
	query struct {
		Term string
	},
) (body templ.Component, err error) {
	_ = query
	return body, err
}
