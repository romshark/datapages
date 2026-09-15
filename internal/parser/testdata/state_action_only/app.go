package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type StateIndex struct {
	Filter string
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTFilter is /filter
//
// The page declares no StreamOpen, StreamClose or OnXXX handler. State alone
// makes the generator emit the stream that allocates and releases the instance.
func (PageIndex) POSTFilter(
	r *http.Request,
	state datapages.State[StateIndex],
) error {
	state.Values.Filter = "all"
	return nil
}
