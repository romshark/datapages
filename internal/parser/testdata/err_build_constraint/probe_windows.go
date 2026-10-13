// PageWin is missing from the model on every other platform.

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// PageWin is /win
type PageWin struct{ App *App }

func (PageWin) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
