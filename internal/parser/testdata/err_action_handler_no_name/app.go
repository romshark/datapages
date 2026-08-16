//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type (
	App struct{}

	// PageIndex is /
	PageIndex struct{ App *App }
)

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(r *http.Request) (body datapages.Component, err error) {
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

// PATCH is /form/patch/{$}
func (PageForm) PATCH(r *http.Request) error {
	_ = r
	return nil
}

// PUT is /form/update/{$}
func (PageForm) PUT(
	r *http.Request,
	sse datapages.SSE,
) error {
	_ = r
	_ = sse
	return nil
}
