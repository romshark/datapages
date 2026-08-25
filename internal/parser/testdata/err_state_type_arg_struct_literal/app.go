package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(
	r *http.Request,
	// The type argument must be a named type, unlike the anonymous structs
	// datapages.Query, Signals and Path accept: the generator derives the
	// state pool and slot symbols from the name.
	state datapages.State[struct{ Filter string }],
) error {
	state.Values.Filter = "all"
	return nil
}
