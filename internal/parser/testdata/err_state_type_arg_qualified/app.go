package app

import (
	"net/http"
	"time"

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
	// The type argument must name a type of the app package.
	// A qualified name from another package is not one.
	state datapages.State[time.Time],
) error {
	return nil
}
