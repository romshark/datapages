//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// POST is /form/{$}
func (PageForm) POST(r *http.Request) error {
	_ = r
	return nil
}

// DELETE is /form/{$}
func (PageForm) DELETE(r *http.Request) error {
	_ = r
	return nil
}

// PUT is /form/update/{$}
func (PageForm) PUT(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
) error {
	_ = r
	_ = sse
	return nil
}
