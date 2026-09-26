// Package app has a global Head that takes a session before the request.
// The page cache also renders documents through Head.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type Session = datapages.Session[struct{}]

type App struct{}

// The parameters are matched by type, so their order and names are free.
func (*App) Head(session Session, req *http.Request) datapages.Head { return nil }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	return nil, nil
}
