package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

type Session struct {
	UserID string
}

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
	newSession Session,
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
	newSession Session,
	redirect string,
	err error,
) {
	return newSession, "/", nil
}

// POSTSignOut is /login/sign-out
//
// Action with closeSession and redirect.
func (PageLogin) POSTSignOut(
	r *http.Request,
) (
	closeSession bool,
	redirect string,
	err error,
) {
	return true, "/", nil
}
