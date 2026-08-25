package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct {
	App *App
	AbstractHelper[int]
}

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageAct is /act
type PageAct[T any] struct{ App *App } /* ErrTypeParams */

func (PageAct[T]) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

type AbstractHelper[T any] struct{ App *App } /* ErrTypeParams */

func (AbstractHelper[T]) StreamOpen(
	r *http.Request, streamID datapages.StreamID,
) error {
	return nil
}
