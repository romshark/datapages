//nolint:all
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return body, err
}

/* ErrEventTargetUserIDsNoSession: TargetUserIDs without Session type */

// EventChat is "chat"
type EventChat struct {
	TargetUserIDs []string `json:"-"`
	Message       string   `json:"message"`
}

// EventPublic is "public"
type EventPublic struct {
	Data string `json:"data"`
}
