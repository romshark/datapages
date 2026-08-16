package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

func (*App) RecoverError(err error, sse datapages.SSE) error { return err }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("index failed")
}

// PageError500 is /whoops
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("error page failed")
}
