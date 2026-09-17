// Package app binds a query parameter to a signal whose name no attribute name
// and no JavaScript identifier can carry. The handler declares no signals parameter,
// which is what leaves nothing to hold the name against.
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
	query datapages.Query[struct {
		Term string `query:"t" reflectsignal:"a\"b"` /* ErrQueryReflectSignalInvalid */
	}],
) (body datapages.Component, err error) {
	_ = query
	return body, err
}
