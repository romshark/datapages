// Package app has pages and a stream embed that embed types other than
// stream embeds, declared in the app package and in other packages.
//
// Go promotes the methods of every embedded type to the page.
// The parser reads handlers from the app package only:
// shared.Likes.POSTLike would be promoted and never served.
package app

import (
	"net/http"
	"strings"

	"datapagestest/fixture/err_embed_not_stream_embed/shared"

	"github.com/romshark/datapages"
)

type App struct{}

// Helper has no App field, which makes it no stream embed.
type Helper struct{}

// Base carries the stream hook the embedding page inherits.
type Base struct {
	App          *App
	shared.Likes // ErrPageHasExtraFields
}

func (Base) StreamOpen(_ *http.Request, _ datapages.StreamID) error { return nil }

// PageIndex is /
type PageIndex struct {
	App *App
	Base
	Helper // ErrPageHasExtraFields
}

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageForm is /form
type PageForm struct {
	App               *App
	shared.Likes      // ErrPageHasExtraFields
	*strings.Builder  // ErrPageHasExtraFields
	shared.Cache[App] // ErrPageHasExtraFields
}

func (PageForm) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
