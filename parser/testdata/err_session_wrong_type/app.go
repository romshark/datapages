//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

/* ErrSessionNotStruct: Session defined as string */

type Session string

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}
