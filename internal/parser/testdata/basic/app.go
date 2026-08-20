package app

import (
	"fmt"
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	switch r.Header.Get("X-Variant") {
	case "A":
		return indexA(), nil
	case "B":
		return indexB(), nil
	}
	return body, fmt.Errorf("unknown page variant")
}

func (*App) Head(r *http.Request) datapages.Head {
	return nil
}

func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return nil
}

// PageError404 is /the-not-found-page
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageError500 is /the-internal-error-page
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageExample is /example
type PageExample struct{ App *App }

func (PageExample) GET(r *http.Request) (
	view datapages.Component,
	meta datapages.Head, err error,
) {
	return view, meta, err
}
