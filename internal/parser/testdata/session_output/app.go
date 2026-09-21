package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// PageIndex is /
type PageIndex struct{ App *App }

// GET without newSession or closeSession.
func (PageIndex) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return body, err
}

// PageLogin is /login
type PageLogin struct{ App *App }

// GET with newSession.
func (PageLogin) GET(
	r *http.Request,
) (
	body datapages.Component,
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
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/"}, nil
}

// PageSignOut is /sign-out
type PageSignOut struct{ App *App }

// GET with closeSession, reading the session it closes.
func (PageSignOut) GET(
	r *http.Request, session Session,
) (
	body datapages.Component,
	closeSession datapages.CloseSession,
	err error,
) {
	return body, datapages.CloseSession(!session.IsGuest()), err
}

// PageLeave is /leave
type PageLeave struct{ App *App }

// GET with closeSession and no session parameter.
func (PageLeave) GET(
	r *http.Request,
) (
	body datapages.Component,
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return body, true, redirect, err
}
