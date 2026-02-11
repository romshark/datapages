package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

// GET without redirect.
func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// PageLogin is /login
type PageLogin struct{ App *App }

// GET with redirect.
func (PageLogin) GET(
	r *http.Request,
) (body templ.Component, redirect string, err error) {
	return body, redirect, err
}

// POSTSignIn is /login/sign-in
//
// Action with redirect and redirectStatus.
func (PageLogin) POSTSignIn(
	r *http.Request,
) (redirect string, redirectStatus int, err error) {
	return "/", 303, nil
}
