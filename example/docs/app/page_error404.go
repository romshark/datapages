package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (
	body datapages.Component, head datapages.Head, err error,
) {
	return pageNotFound(), headNotFound(), nil
}
