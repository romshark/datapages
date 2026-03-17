//nolint:all
package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
)

type App struct{}

type Session struct {
	UserID   string
	IssuedAt time.Time
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrEventSubjectSignalInvalid: signal tag with invalid name */

// EventBad is "bad"
type EventBad struct {
	SubjectInstance string `json:"-" signal:"has spaces"`

	Data string `json:"data"`
}
