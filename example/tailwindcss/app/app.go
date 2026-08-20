package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

func (*App) Head(r *http.Request) datapages.Head {
	return head()
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageIndex(), nil
}
