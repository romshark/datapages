//nolint:all
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

/* ErrEventSubjectJSONExcluded: subject field tagged json:"-" */

// EventBad is "bad"
type EventBad struct {
	Room datapages.Subject `signal:"room" json:"-"`

	Data string `json:"data"`
}
