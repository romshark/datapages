package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

	"github.com/a-h/templ"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request, session Session) (
	body templ.Component,
	redirect string,
	disableRefreshAfterHidden bool,
	err error,
) {
	if session.UserID != "" {
		// Already logged in
		return nil, href.Index(), false, nil
	}
	return pageLogin(false), redirect, true, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	signals struct {
		EmailOrUsername string `json:"emailorusername"`
		Password        string `json:"password"`
	},
) (
	body templ.Component,
	redirect string,
	redirectStatus int,
	newSession Session,
	err error,
) {
	if session.UserID != "" {
		// Already logged in.
		redirect, redirectStatus = href.Index(), http.StatusSeeOther
		return
	}
	uid, err := p.App.repo.Login(signals.EmailOrUsername, signals.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) ||
			errors.Is(err, domain.ErrUserNotFound) {
			p.App.LoginSubmissions.WithLabelValues("failure").Inc()
			// Re-render page with feedback
			err, body = nil, pageLogin(true)
		}
		return
	}

	p.App.LoginSubmissions.WithLabelValues("success").Inc()
	now := time.Now()
	newSession = Session{
		UserID:   uid,
		IssuedAt: now,
	}
	redirect, redirectStatus = href.Index(), http.StatusSeeOther
	return
}
