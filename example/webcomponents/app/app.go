package app

import (
	"embed"
	"net/http"

	"github.com/a-h/templ"
)

//go:embed static/*
var StaticFS embed.FS

type App struct{}

func (*App) Head(r *http.Request) templ.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return pageIndex(), nil
}
