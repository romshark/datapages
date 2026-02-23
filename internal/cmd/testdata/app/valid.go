package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
)

type App struct{}

type Session struct {
	UserID   string
	IssuedAt time.Time
}

func (*App) Head(r *http.Request) (head templ.Component, err error) {
	return nil, nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request, session Session) (body templ.Component, err error) {
	return nil, nil
}
