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

// PageNum is /num
type PageNum int /* ErrPageNotStruct */

func (PageNum) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageAlias is /alias
type PageAlias = pageAliased /* ErrPageNotStruct */

type pageAliased struct{ App *App }

func (pageAliased) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
