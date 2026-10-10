package app

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/templates"
)

// immutable caches an image for a year: the bytes of a URL path never change.
var immutable = datapages.FileCache{MaxAge: 365 * 24 * time.Hour, Immutable: true}

// picture is the File body of a template picture,
// which the program holds in memory and has nothing to close.
type picture struct{ *bytes.Reader }

func (picture) Close() error { return nil }

// GETImage is /images/{name}
//
// Serves the pictures of the templates from the program and uploads from the
// store. The route is [images.Prefix] followed by the name.
//
// This example authenticates nobody: an image is readable by whoever knows its
// URL path, the SHA-256 of its bytes.
func (a *App) GETImage(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (datapages.File, error) {
	src := images.Prefix + path.Values.Name
	if data, ok := templates.Picture(src); ok {
		return datapages.File{
			Type:  images.ContentType(src),
			Body:  picture{bytes.NewReader(data)},
			Cache: immutable,
		}, nil
	}
	f, contentType, err := a.images.Open(src)
	switch {
	case errors.Is(err, images.ErrNotFound):
		return datapages.File{}, datapages.ErrNotFound
	case err != nil:
		return datapages.File{}, err
	}
	return datapages.File{Type: contentType, Body: f, Cache: immutable}, nil
}
