package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (
	body templ.Component, err error,
	err2 error, /* ErrSignatureMultiErrRet */
) {
	return body, err, err2
}

// PageNoAppField is /no-app-field
type PageNoAppField struct {
	/* ErrPageMissingFieldApp */
}

func (PageNoAppField) GET() (body templ.Component, err error) { return body, err }

// PageNoGET is /this-page-is-missing-a-get-handler
type PageNoGET struct{ App *App }

/* ErrPageMissingGET */

// PageHasExtraFields is /has-extra-fields
type PageHasExtraFields struct {
	App         *App
	Unsupported int // ErrPageHasExtraFields
}

func (PageHasExtraFields) GET(
/* ErrSignatureMissingReq */
) (body templ.Component, err error) {
	return body, err
}

// Page is /has-invalid-name
type Page struct{ App *App }

func (Page) GET(r *http.Request) (body templ.Component, err error) { return body, err }

// Page404 is /has-invalid-name1
type Page404 struct{ App *App }

func (Page404) GET(r *http.Request) (body templ.Component, err error) { return body, err }

// Page_invalid is /has-invalid-name2
type Page_invalid struct{ App *App }

func (Page_invalid) GET(r *http.Request) (body templ.Component, err error) { return body, err }

// Paged is /has-invalid-name3
type Paged struct{ App *App }

func (Paged) GET(r *http.Request) (body templ.Component, err error) { return body, err }

/* ErrPageInvalidPathComm */

// PageBadPath is foo
type PageBadPath struct{ App *App }

func (PageBadPath) GET(r *http.Request) (body templ.Component, err error) { return body, err }

/* ErrPageMissingPathComm */

type PageWithoutComment struct{ App *App }

func (PageWithoutComment) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageActionTest is /action-test
type PageActionTest struct{ App *App }

/* ErrPageMissingGET */

/* ErrActionMissingPathComm */

func (PageActionTest) POSTNoComment(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrActionNameInvalid */

// POST404 is /action-test/404
func (PageActionTest) POST404(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrActionInvalidPathComm */

// POSTCommInvalid handles /other
func (PageActionTest) POSTCommInvalid(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrActionPathNotUnderPage */

// POSTOutside is /other
func (PageActionTest) POSTOutside(r *http.Request) (body templ.Component, err error) {
	return body, err
}
