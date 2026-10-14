//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// POSTSave is /save
func (PageIndex) POSTSave(r *http.Request, sse datapages.SSE) error {
	return nil
}

/* ErrActionStreamConfigNoAction: PageIndex declares no POSTMissing */

func (PageIndex) StreamConfigPOSTMissing() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// POSTPlain is /plain
func (PageIndex) POSTPlain(r *http.Request) error {
	return nil
}

/* ErrActionStreamConfigNoStream: POSTPlain answers with no stream */

func (PageIndex) StreamConfigPOSTPlain() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// POSTRedirect is /redirect
func (PageIndex) POSTRedirect(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (redirect datapages.Redirect, err error) {
	return redirect, err
}

/* ErrActionStreamConfigNoStream: the redirect carries the page cache writes */

func (PageIndex) StreamConfigPOSTRedirect() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

/* ErrActionStreamConfigInput: signals POSTSave does not read */

func (PageIndex) StreamConfigPOSTSave(
	r *http.Request,
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
) error {
	return nil
}

/* ErrActionStreamConfigInput: a path of another type than PUTItem's */

func (PageIndex) StreamConfigPUTItem(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"id"`
	}],
) (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

// PATCHStatic is /static
func (PageIndex) PATCHStatic(r *http.Request, sse datapages.SSE) error {
	return nil
}

/* ErrStreamConfigSignature: static method with error */

func (PageIndex) StreamConfigPATCHStatic() (datapages.StreamConfig, error) {
	return datapages.StreamConfig{}, nil
}

/* ErrActionStreamConfigNoAction: App declares no POSTGone */

func (*App) StreamConfigPOSTGone() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// Shared is a stream embed.
type Shared struct{ App *App }

func (Shared) StreamOpen(r *http.Request, streamID datapages.StreamID) error {
	return nil
}

/* ErrStreamEmbedUnsupportedMethod: a stream embed has no actions */

func (Shared) StreamConfigPOSTSave() datapages.StreamConfig {
	return datapages.StreamConfig{}
}

// PageShared is /shared
type PageShared struct {
	App *App
	Shared
}

func (PageShared) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}
