package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageMissingReq is /missing-req
type PageMissingReq struct{ App *App }

/* ErrSignatureMissingReq */

func (PageMissingReq) GET() (body datapages.Component, err error) {
	return body, err
}

// PageMultiErrRet is /multi-err-ret
type PageMultiErrRet struct{ App *App }

func (PageMultiErrRet) GET(r *http.Request) (
	body datapages.Component, err error,
	err2 error, /* ErrSignatureMultiErrRet */
) {
	return body, err, err2
}

// PageUnknownInput is /unknown-input
type PageUnknownInput struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageUnknownInput) GET(
	r *http.Request, unknown int, /* this is the error */
) (body datapages.Component, err error) {
	return body, err
}

// PageDuplicateReq is /duplicate-req
type PageDuplicateReq struct{ App *App }

/* ErrSignatureUnsupportedInput */

func (PageDuplicateReq) GET(
	r, a *http.Request, /* second *http.Request is unsupported */
) (body datapages.Component, err error) {
	return body, err
}

// PageMultiUnsupported is /multi-unsupported
type PageMultiUnsupported struct{ App *App }

/* ErrSignatureUnsupportedInput (x2) */

func (PageMultiUnsupported) GET(
	r *http.Request, asd, asd2 int,
) (body datapages.Component, err error) {
	return body, err
}

// PageMissingBody is /missing-body
type PageMissingBody struct{ App *App }

/* ErrSignatureGETMissingBody */

func (PageMissingBody) GET(r *http.Request) (err error) {
	return err
}

// PageTwoBodies is /two-bodies
type PageTwoBodies struct{ App *App }

/* ErrSignatureDuplicateOutput */

func (PageTwoBodies) GET(r *http.Request) (
	body datapages.Component,
	second datapages.Component,
	err error,
) {
	return body, second, err
}

// PageTwoHeads is /two-heads
type PageTwoHeads struct{ App *App }

/* ErrSignatureDuplicateOutput */

func (PageTwoHeads) GET(r *http.Request) (
	body datapages.Component,
	head datapages.Head,
	second datapages.Head,
	err error,
) {
	return body, head, second, err
}
