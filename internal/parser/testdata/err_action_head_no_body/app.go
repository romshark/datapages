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

/* ErrSignatureActionHeadWithoutBody: the head has no response to travel in */

// POSTTitle is /title
func (PageIndex) POSTTitle(r *http.Request) (head datapages.Head, err error) {
	return head, err
}
