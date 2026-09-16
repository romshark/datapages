package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateIndex struct{ Count int }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(
	r *http.Request,
	state datapages.State[StateIndex],
	stateID int,
) error {
	return nil
}
