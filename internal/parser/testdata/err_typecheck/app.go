// This fixture MUST not type-check.
// Declaring undefinedHelper makes TestParse_ErrTypeCheck prove nothing.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	// This syntax error is expected! DO NOT FIX.
	return undefinedHelper(), nil
}
