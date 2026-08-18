//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrEventSubjectUserNoSession: user-addressed event without Session type */

// EventChat is "chat"
type EventChat struct {
	Recipient datapages.SubjectUser

	Message string `json:"message"`
}

// EventPublic is "public"
type EventPublic struct {
	Data string `json:"data"`
}
