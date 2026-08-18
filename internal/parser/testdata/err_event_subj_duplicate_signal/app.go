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

/* ErrEventSubjectDuplicateSignal: two subject fields with the same signal tag */

// EventBad is "bad"
type EventBad struct {
	Foo datapages.Subject `signal:"instance_id"`
	Bar datapages.Subject `signal:"instance_id"`

	Message string `json:"message"`
}
