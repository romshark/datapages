//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrEventSubjectDuplicateSignal: two subject fields with the same signal tag */

// EventBad is "bad"
type EventBad struct {
	SubjectFoo []string `signal:"instance_id"`
	SubjectBar []string `signal:"instance_id"`

	Message string `json:"message"`
}
