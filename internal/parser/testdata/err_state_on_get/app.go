package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateIndex struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
	state datapages.State[StateIndex], // not allowed on GET
) (body datapages.Component, err error) {
	return nil, nil
}

// POSTDummy is /dummy
func (PageIndex) POSTDummy(
	r *http.Request,
	state datapages.State[StateIndex],
) error {
	return nil
}
