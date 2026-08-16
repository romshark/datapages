package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

// GET without redirect.
func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// PageLogin is /login
type PageLogin struct{ App *App }

// GET with redirect.
func (PageLogin) GET(
	r *http.Request,
) (body datapages.Component, redirect datapages.Redirect, err error) {
	return body, redirect, err
}

// POSTSignIn is /login/sign-in
//
// Action with redirect.
func (PageLogin) POSTSignIn(
	r *http.Request,
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: "/", Status: http.StatusSeeOther}, nil
}
