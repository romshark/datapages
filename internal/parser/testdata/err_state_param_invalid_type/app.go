package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// StateIndex is named like a state type but is not a struct.
type StateIndex int

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(
	r *http.Request,
	state datapages.State[StateIndex],
) error {
	return nil
}
