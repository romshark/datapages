// Package app has path and query fields of types it names itself. Every one of them
// takes a conversion in the generated code, since strconv hands back the basic type.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type (
	UserID  uint64
	Level   int
	Ratio   float64
	Ratio32 float32
	Flag    bool
	Slug    string
)

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageKinds is /kinds/{id}/{level}/{ratio}/{flag}/{slug}
type PageKinds struct{ App *App }

func (PageKinds) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID    UserID `path:"id"`
		Level Level  `path:"level"`
		Ratio Ratio  `path:"ratio"`
		Flag  Flag   `path:"flag"`
		Slug  Slug   `path:"slug"`
	}],
	query datapages.Query[struct {
		ID    UserID  `query:"id"`
		Level Level   `query:"level"`
		Ratio Ratio32 `query:"ratio"`
		Flag  Flag    `query:"flag"`
		Slug  Slug    `query:"slug"`
	}],
) (body datapages.Component, err error) {
	return nil, nil
}
