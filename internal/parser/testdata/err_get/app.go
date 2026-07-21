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

// PageMissingReq is /missing-req
type PageMissingReq struct{ App *App }

/* ErrSignatureMissingReq */

func (PageMissingReq) GET() (body templ.Component, err error) {
	return body, err
}

// PageMultiErrRet is /multi-err-ret
type PageMultiErrRet struct{ App *App }

func (PageMultiErrRet) GET(r *http.Request) (
	body templ.Component, err error,
	err2 error, /* ErrSignatureMultiErrRet */
) {
	return body, err, err2
}

// PageUnknownInput is /unknown-input
type PageUnknownInput struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageUnknownInput) GET(
	r *http.Request, unknown int, /* this is the error */
) (body templ.Component, err error) {
	return body, err
}

// PageDuplicateReq is /duplicate-req
type PageDuplicateReq struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageDuplicateReq) GET(
	r, a *http.Request, /* second *http.Request is unsupported */
) (body templ.Component, err error) {
	return body, err
}

// PageMultiUnsupported is /multi-unsupported
type PageMultiUnsupported struct{ App *App }

/* ErrSignatureUnsupportedInput (x2) */

func (PageMultiUnsupported) GET(
	r *http.Request, asd, asd2 int,
) (body templ.Component, err error) {
	return body, err
}

// PageMissingBody is /missing-body
type PageMissingBody struct{ App *App }

/* ErrSignatureGETMissingBody */

func (PageMissingBody) GET(r *http.Request) (err error) {
	return err
}

// PageBodyWrongName is /body-wrong-name
type PageBodyWrongName struct{ App *App }

/* ErrSignatureGETBodyWrongName */

func (PageBodyWrongName) GET(r *http.Request) (content templ.Component, err error) {
	return content, err
}

// PageHeadWrongName is /head-wrong-name
type PageHeadWrongName struct{ App *App }

/* ErrSignatureGETHeadWrongName */

func (PageHeadWrongName) GET(r *http.Request) (
	body templ.Component,
	header templ.Component, /* should be "head" not "header" */
	err error,
) {
	return body, header, err
}
