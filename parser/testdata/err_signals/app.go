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

/* ErrSignalsParamNotStruct */

func (PageNotStruct) GET(r *http.Request, signals int) (body templ.Component, err error) {
	_ = signals
	return body, err
}

// PageUnexported is /unexported
type PageUnexported struct{ App *App }

/* ErrSignalsFieldUnexported */

func (PageUnexported) GET(
	r *http.Request,
	signals struct {
		name string `json:"name"`
	},
) (body templ.Component, err error) {
	_ = signals
	return body, err
}

// PageMissingTag is /missing-tag
type PageMissingTag struct{ App *App }

/* ErrSignalsFieldMissingTag */

func (PageMissingTag) GET(
	r *http.Request,
	signals struct {
		Name string
	},
) (body templ.Component, err error) {
	_ = signals
	return body, err
}

// PageBadReflect is /bad-reflect
type PageBadReflect struct{ App *App }

/* ErrQueryReflectSignalNotInSignals */

func (PageBadReflect) GET(
	r *http.Request,
	query struct {
		Term string `query:"t" reflectsignal:"nonexistent"`
	},
	signals struct {
		Filter string `json:"filter"`
	},
) (body templ.Component, err error) {
	_ = query
	_ = signals
	return body, err
}
