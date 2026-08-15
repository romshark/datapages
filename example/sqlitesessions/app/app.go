package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/sqlitesessions/app/userstore"
	"github.com/romshark/datapages/example/sqlitesessions/datapagesgen/href"
)

// EventSessionClosed is "sessions.closed"
type EventSessionClosed struct {
	SubjectUser []string
	Token       string `json:"token"`
}

// SessionData is what this application keeps in the session on top of the
// fields datapages provides (UserID and IssuedAt).
type SessionData struct {
	Name  string
	Email string
}

type Session = datapages.Session[SessionData]

type User = userstore.User

type App struct {
	users *userstore.Store
}

func NewApp(users *userstore.Store) *App {
	return &App{users: users}
}

func (*App) Head(r *http.Request) templ.Component { return head() }

// POSTSignOut is /signout/{$}
//
// Dispatch must happen before returning closeSession=true, so
// subscribed tabs receive the event before the framework tears down
// this session's SSE connection.
func (*App) POSTSignOut(
	r *http.Request,
	session Session,
	dispatch func(EventSessionClosed) error,
) (
	closeSession bool, redirect string, err error,
) {
	if !session.IsGuest() {
		_ = dispatch(EventSessionClosed{
			SubjectUser: []string{session.UserID()},
			Token:       session.Token(),
		})
	}
	return true, href.PageIndex(), nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request, session Session) (
	body, head templ.Component, err error,
) {
	users, err := p.App.users.ListUsers(r.Context())
	if err != nil {
		return nil, nil, fmt.Errorf("listing users: %w", err)
	}
	return pageIndex(session, users), pageIndexHead(), nil
}

// OnSessionClosed redirects tabs sharing the closed cookie so they
// re-fetch PageIndex as a guest.
func (p PageIndex) OnSessionClosed(
	event EventSessionClosed,
	sse datapages.SSE,
	session Session,
) error {
	if event.Token != session.Token() {
		return nil
	}
	return sse.Redirect(href.PageIndex())
}

// validateLogin returns "" if the input is acceptable, otherwise a
// user-facing error message. Shared by POSTValidate and POSTSubmit.
func validateLogin(email, password string) string {
	if strings.TrimSpace(email) == "" {
		return "Email is required"
	}
	if password == "" {
		return "Password is required"
	}
	return ""
}

func validateRegister(name, email, password string) string {
	if strings.TrimSpace(name) == "" {
		return "Name is required"
	}
	if strings.TrimSpace(email) == "" {
		return "Email is required"
	}
	if !strings.Contains(email, "@") {
		return "Email must contain an @"
	}
	if len(password) < 8 {
		return "Password must be at least 8 characters"
	}
	return ""
}

// PageLogin is /login
type PageLogin struct{ App *App }

func (p PageLogin) GET(r *http.Request, session Session) (
	body, head templ.Component, redirect string, err error,
) {
	if !session.IsGuest() {
		return nil, nil, href.PageIndex(), nil
	}
	return pageLogin("", false), pageLoginHead(), "", nil
}

// POSTValidate is /login/validate
//
// Fired on every keystroke via data-on:input for live feedback.
func (p PageLogin) POSTValidate(
	r *http.Request,
	session Session,
	signals struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) (body templ.Component, err error) {
	msg := validateLogin(signals.Email, signals.Password)
	return pageLogin(msg, msg == ""), nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	signals struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) (
	body templ.Component,
	redirect string,
	redirectStatus int,
	newSession datapages.NewSession[SessionData],
	err error,
) {
	if !session.IsGuest() {
		redirect, redirectStatus = href.PageIndex(), http.StatusSeeOther
		return
	}
	if msg := validateLogin(signals.Email, signals.Password); msg != "" {
		body, err = pageLogin(msg, false), nil
		return
	}
	user, err := p.App.users.Authenticate(r.Context(), signals.Email, signals.Password)
	if err != nil {
		if errors.Is(err, userstore.ErrInvalidCredentials) {
			body, err = pageLogin("Invalid email or password", false), nil
			return
		}
		return
	}
	newSession = datapages.NewSession[SessionData]{
		UserID: user.ID,
		Data:   SessionData{Name: user.Name, Email: user.Email},
	}
	redirect, redirectStatus = href.PageIndex(), http.StatusSeeOther
	return
}

// PageRegister is /register
type PageRegister struct{ App *App }

func (p PageRegister) GET(r *http.Request, session Session) (
	body, head templ.Component, redirect string, err error,
) {
	if !session.IsGuest() {
		redirect = href.PageIndex()
		return
	}
	return pageRegister("", false), pageRegisterHead(), "", nil
}

// POSTValidate is /register/validate
//
// Fired on every keystroke via data-on:input for live feedback.
func (p PageRegister) POSTValidate(
	r *http.Request,
	session Session,
	signals struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) (body templ.Component, err error) {
	msg := validateRegister(signals.Name, signals.Email, signals.Password)
	return pageRegister(msg, msg == ""), nil
}

// POSTSubmit is /register/submit
func (p PageRegister) POSTSubmit(
	r *http.Request,
	session Session,
	signals struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) (
	body templ.Component,
	redirect string,
	redirectStatus int,
	newSession datapages.NewSession[SessionData],
	err error,
) {
	if !session.IsGuest() {
		redirect, redirectStatus = href.PageIndex(), http.StatusSeeOther
		return
	}
	if msg := validateRegister(
		signals.Name, signals.Email, signals.Password,
	); msg != "" {
		body, err = pageRegister(msg, false), nil
		return
	}
	user, err := p.App.users.Register(
		r.Context(), signals.Name, signals.Email, signals.Password,
	)
	if err != nil {
		if errors.Is(err, userstore.ErrEmailAlreadyInUse) {
			body, err = pageRegister("That email is already registered", false), nil
			return
		}
		return
	}
	newSession = datapages.NewSession[SessionData]{
		UserID: user.ID,
		Data:   SessionData{Name: user.Name, Email: user.Email},
	}
	redirect, redirectStatus = href.PageIndex(), http.StatusSeeOther
	return
}
