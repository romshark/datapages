// Package app has a global Head that takes a session.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type Session = datapages.Session[struct{}]

type App struct{}

func (*App) Head(r *http.Request, session Session) datapages.Component { return nil }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
