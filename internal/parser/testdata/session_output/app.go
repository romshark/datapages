package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

// GET without newSession or closeSession.
func (PageIndex) GET(
	r *http.Request,
) (body templ.Component, err error) {
	return body, err
}

// PageLogin is /login
type PageLogin struct{ App *App }

// GET with newSession.
func (PageLogin) GET(
	r *http.Request,
) (
	body templ.Component,
	newSession datapages.NewSession[struct{}],
	err error,
) {
	return body, newSession, err
}

// POSTSubmit is /login/submit
//
// Action with newSession and redirect.
func (PageLogin) POSTSubmit(
	r *http.Request,
) (
	newSession datapages.NewSession[struct{}],
	redirect datapages.Redirect,
	err error,
) {
	return newSession, datapages.Redirect{URL: "/"}, nil
}

// POSTSignOut is /login/sign-out
//
// Action with closeSession and redirect.
func (PageLogin) POSTSignOut(
	r *http.Request,
) (
	closeSession bool,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/"}, nil
}
