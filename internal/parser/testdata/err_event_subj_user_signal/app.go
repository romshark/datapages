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

func (PageIndex) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, err error) {
	_ = session
	return body, err
}

/* ErrEventSubjectUserSignal: user-addressed subject field with a signal tag */

// EventBad is "bad"
type EventBad struct {
	Recipient datapages.SubjectUser `signal:"user_id"`

	Data string `json:"data"`
}
