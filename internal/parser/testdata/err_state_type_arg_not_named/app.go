package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateIndex struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(
	r *http.Request,
	// The type argument names the state type itself,
	// never a pointer to it: datapages.State[StateIndex].
	state datapages.State[*StateIndex],
) error {
	return nil
}
