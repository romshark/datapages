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

/* ErrEventSubjectSignalInvalid: signal tag with invalid name */

// EventBad is "bad"
type EventBad struct {
	SubjectInstance string `signal:"has spaces"`

	Data string `json:"data"`
}
