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
	return page(r.PathValue("some_runtime_value")), nil
}

const (
	ConstantStringOK    = "https://data-star.dev"
	ConstantStringNOTOK = "/c"
	InternalConst       = "/internal"
)

func loginHref() templ.SafeURL      { return "/" }
func someOtherFunc() templ.SafeURL  { return "" }
func buildURL(id int) templ.SafeURL { _ = id; return "" }

var id = 1
