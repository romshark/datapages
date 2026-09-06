package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTGoStream is /go-stream
func (*App) POSTGoStream(
	_ *http.Request,
	sse datapages.SSE, /* ErrSSEOnAppMethod */
) (redirect datapages.Redirect, err error) {
	_ = sse
	return redirect, nil
}
