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

/* ErrBodyWithSSE: body with sse */

// POSTRender is /render
func (PageIndex) POSTRender(
	r *http.Request,
	sse datapages.SSE,
) (body datapages.Component, err error) {
	_ = sse
	return body, nil
}

/* ErrBodyWithSSE: body and head with sse */

// POSTRenderWithHead is /render-with-head
func (PageIndex) POSTRenderWithHead(
	r *http.Request,
	sse datapages.SSE,
) (body datapages.Component, head datapages.Head, err error) {
	_ = sse
	return body, head, nil
}
