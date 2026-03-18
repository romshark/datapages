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

/* ErrEventSubjectDuplicateSignal: two subject fields with the same signal tag */

// EventBad is "bad"
type EventBad struct {
	SubjectFoo []string `json:"-" signal:"instance_id"`
	SubjectBar []string `json:"-" signal:"instance_id"`

	Message string `json:"message"`
}
