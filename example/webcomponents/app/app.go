package app

import (
	"embed"
	"net/http"

	"github.com/romshark/datapages"
)

//go:embed static/*
var StaticFS embed.FS

type App struct{}

func (*App) Head(r *http.Request) datapages.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageIndex(), nil
}
