//nolint:all

package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

/* ErrSessionMissingIssuedAt: has UserID but no IssuedAt */

type Session struct {
	UserID string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}
