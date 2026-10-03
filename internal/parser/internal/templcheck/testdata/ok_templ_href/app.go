//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

const ExternalConst = "https://data-star.dev"

// shadowExternalConst declares a constant with the name of the package-level one
// a template href uses. The href is checked against the package-level value.
func shadowExternalConst() string {
	const ExternalConst = "/relative"
	return ExternalConst
}

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return page(), nil
}
