package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

/* ErrAppRecoverErrorInvalidSignature */

func (*App) RecoverError(err error, sse datapages.SSE) {
}
