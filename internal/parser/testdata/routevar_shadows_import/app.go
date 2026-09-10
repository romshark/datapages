//nolint:all

// Package app names every route wildcard after an identifier the generated
// href and action packages resolve at package scope. A parameter of that name
// shadows the reference for the whole function body: url.PathEscape(url) then
// reads its own parameter and strings.Builder names no type.
package app

import (
	"net/http"
	"strings"

	"github.com/romshark/datapages"
)

type App struct{}

// Slug marshals itself, which is what sends a path value through textOf.
type Slug string

func (s Slug) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(string(s))), nil
}

func (s *Slug) UnmarshalText(b []byte) error {
	*s = Slug(b)
	return nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageShadowA is /a/{url}/{strings}/{strconv}/{textOf}
//
// The int reaches strconv.FormatInt and the Slug reaches textOf. Every
// qualifier a path value can be written through is covered here and in
// [PageShadowB], each under a parameter that would shadow it.
type PageShadowA struct{ App *App }

func (PageShadowA) GET(
	r *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings Slug   `path:"strings"`
		Strconv int    `path:"strconv"`
		TextOf  Slug   `path:"textOf"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// POSTSave is /a/{url}/{strings}/{strconv}/{textOf}/save
func (PageShadowA) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings Slug   `path:"strings"`
		Strconv int    `path:"strconv"`
		TextOf  Slug   `path:"textOf"`
	}],
) error {
	_ = path
	return nil
}

// PageShadowB is /b/{fmt}/{encoding}/{actionexpr}
//
// No path value is written through these three. They are bound all the same:
// "fmt" and "encoding" by goimports for textOf, "actionexpr" by the action
// writers around every call they build.
type PageShadowB struct{ App *App }

func (PageShadowB) GET(
	r *http.Request,
	path datapages.Path[struct {
		Fmt        string `path:"fmt"`
		Encoding   string `path:"encoding"`
		Actionexpr string `path:"actionexpr"`
	}],
) (body datapages.Component, err error) {
	_ = path
	return body, err
}

// POSTSave is /b/{fmt}/{encoding}/{actionexpr}/save
func (PageShadowB) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		Fmt        string `path:"fmt"`
		Encoding   string `path:"encoding"`
		Actionexpr string `path:"actionexpr"`
	}],
) error {
	_ = path
	return nil
}

// PageShadowQuery is /q/{url}/{strings}
//
// The path-and-query writers declare more locals than the path-only ones and
// name each around whatever the route brought.
type PageShadowQuery struct{ App *App }

func (PageShadowQuery) GET(
	r *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings string `path:"strings"`
	}],
	query datapages.Query[struct {
		Term string `query:"term"`
		Page int    `query:"page"`
	}],
) (body datapages.Component, err error) {
	_, _ = path, query
	return body, err
}

// POSTSave is /q/{url}/{strings}/save
func (PageShadowQuery) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		URL     string `path:"url"`
		Strings string `path:"strings"`
	}],
	query datapages.Query[struct {
		Term string `query:"term"`
		Page int    `query:"page"`
	}],
) error {
	_, _ = path, query
	return nil
}
