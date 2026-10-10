//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

/* ErrFileOnPageGET */

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (
	body datapages.Component, file datapages.File, err error,
) {
	return body, file, err
}

/* ErrFileWithOutput: a body next to the file */

// POSTBody is /body
func (PageIndex) POSTBody(r *http.Request) (
	body datapages.Component, file datapages.File, err error,
) {
	return body, file, nil
}

/* ErrFileWithSSE */

// POSTStream is /stream
func (PageIndex) POSTStream(
	r *http.Request, sse datapages.SSE,
) (file datapages.File, err error) {
	return file, nil
}

/* ErrFileWithPageCache */

// POSTCache is /cache
func (PageIndex) POSTCache(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (file datapages.File, err error) {
	return file, nil
}

/* ErrSignatureDuplicateOutput: two files */

// POSTTwo is /two
func (PageIndex) POSTTwo(r *http.Request) (a, b datapages.File, err error) {
	return a, b, nil
}

/* ErrEnableBgStreamNotGET: a GET action is not the GET of the page */

// GETStreaming is /streaming
func (PageIndex) GETStreaming(r *http.Request) (
	file datapages.File,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return file, false, nil
}
