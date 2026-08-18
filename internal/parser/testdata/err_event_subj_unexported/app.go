//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrEventFieldUnexported: unexported subject field */

// EventBad is "bad"
type EventBad struct {
	room datapages.Subject

	Data string `json:"data"`
}
