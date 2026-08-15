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

/* ErrUnsupportedMethod */

func (PageIndex) Foo() {}

/* ErrUnsupportedMethod */

func (PageIndex) Helper(x int) int { return x }

// unexported methods are fine
func (PageIndex) helper() {}
