package app

import (
	"datapages/app/domain"
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request, session SessionJWT) (
	body templ.Component, redirect Redirect, err error,
) {
	if session.UserID != "" {
		// Already logged in
		return nil, Redirect{Target: "/"}, nil
	}
	return pageLogin(false), redirect, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session SessionJWT,
	signals struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) (
	body templ.Component,
	redirect Redirect,
	newSession SessionJWT,
	err error,
) {
	if session.UserID != "" {
		// Already logged in.
		redirect = Redirect{Target: "/", Status: http.StatusSeeOther}
		return
	}
	uid, err := p.App.repo.Login(signals.Email, signals.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			// Re-render page with feedback
			body = pageLogin(true)
			return
		}
	}

	now := time.Now()
	newSession = SessionJWT{
		UserID:     uid,
		IssuedAt:   now,
		Expiration: now.Add(24 * time.Hour),
	}
	redirect = Redirect{Target: "/", Status: http.StatusSeeOther}
	return
}
