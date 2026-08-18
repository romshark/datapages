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

/* ErrEventSubjectPrefixedField: field named like a subject field but typed
   as a payload field, which is what a pre-typed-subjects app looks like */

// EventBad is "bad"
type EventBad struct {
	SubjectUser []string `json:"subject_user"`

	Data string `json:"data"`
}
