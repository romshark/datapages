package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

/* ErrAppHeadUnsupportedInput */

func (*App) Head(r *http.Request, foo int) templ.Component { return nil }
