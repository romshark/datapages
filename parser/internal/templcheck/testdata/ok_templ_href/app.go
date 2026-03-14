//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

const ExternalConst = "https://data-star.dev"

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return page(), nil
}
