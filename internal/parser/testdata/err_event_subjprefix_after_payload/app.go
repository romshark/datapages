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

func (PageIndex) GET(
	r *http.Request,
	session Session,
) (body templ.Component, err error) {
	_ = session
	return body, err
}

/* ErrEventSubjectAfterPayload: Subject after payload field */

// EventBad is "bad"
type EventBad struct {
	Message     string `json:"message"`
	SubjectUser []string
}
