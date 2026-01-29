package app

import "net/http"

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

func (BaseA) GET(r *http.Request) error { return nil }
func (BaseB) GET(r *http.Request) error { return nil }
