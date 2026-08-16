package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct {
	App *App
	BaseA
	BaseB /* ErrPageConflictingGETEmbed */
}

type (
	BaseA struct{ App *App }
	BaseB struct{ App *App }
)

func (BaseA) GET(r *http.Request) (body datapages.Component, err error) { return nil, nil }
func (BaseB) GET(r *http.Request) (body datapages.Component, err error) { return nil, nil }
