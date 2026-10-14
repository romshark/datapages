//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSave is /save
func (PageIndex) POSTSave(r *http.Request, sse datapages.SSE) error {
	return nil
}

func (PageIndex) StreamConfigPOSTSave() datapages.StreamConfig {
	return datapages.StreamConfig{
		Compression: &datapages.SSECompressionConfig{Disabled: true},
	}
}

// Declared before its action, and with a session the action does not take.
func (PageIndex) StreamConfigPUTItem(
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		Lite bool `json:"lite"`
	}],
) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PUTItem is /item/{id}
func (PageIndex) PUTItem(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Lite bool `json:"lite"`
	}],
) error {
	return nil
}

// POSTPlain is /plain
func (PageIndex) POSTPlain(r *http.Request, sse datapages.SSE) error {
	return nil
}

// POSTClear is /clear
func (*App) POSTClear(r *http.Request, pageCache datapages.PageCacheWriter) error {
	return nil
}

func (*App) StreamConfigPOSTClear(r *http.Request) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}
