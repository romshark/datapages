package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	switch r.Header.Get("X-Variant") {
	case "A":
		return indexA(), nil
	case "B":
		return indexB(), nil
	}
	return body, fmt.Errorf("unknown page variant")
}

func (*App) Head(r *http.Request) (body templ.Component, err error) {
	return body, err
}

func (*App) Recover500(
	err error,
	sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageError404 is /the-not-found-page
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageError500 is /the-internal-error-page
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

// PageExample is /example
type PageExample struct{ App *App }

func (PageExample) GET(r *http.Request) (body, head templ.Component, err error) {
	return body, head, err
}
