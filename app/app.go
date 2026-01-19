package app

import (
	"context"
	"datapages/app/domain"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type Redirect struct {
	Target string
	Status int
}

type SessionJWT struct {
	UserID     string    `json:"sub"` // Subject.
	IssuedAt   time.Time `json:"iat"`
	Expiration time.Time `json:"exp"`
	Issuer     string    `json:"iss"`
}

type App struct {
	// db, etc.
	repo *domain.Repository
}

func NewApp(repo *domain.Repository) *App { return &App{repo: repo} }

type SearchParams struct {
	Term     string `json:"term" query:"t" reflectsignal:"term"`
	Category string `json:"category" query:"c" reflectsignal:"category"`
	PriceMin int64  `json:"pmin,omitempty" query:"pmin" reflectsignal:"pmin"`
	PriceMax int64  `json:"pmax,omitempty" query:"pmax" reflectsignal:"pmax"`
	Location string `json:"location" query:"l" reflectsignal:"location"`
}

// Page render funcs
func (*App) Head(r *http.Request) (body templ.Component, err error) {
	return head(), nil
}

type Chat struct {
	ID                      string
	Title                   string
	PostID                  string
	PostSlug                string
	UnreadMessages          int
	LastMessageSenderUserName string
	LastMessageText         string
}

// Base is the main page wrapper
type Base struct{ App *App }

type baseData struct {
	UnreadChats int
}

func (b Base) baseData(
	ctx context.Context, session SessionJWT,
) (baseData, error) {
	if session.UserID == "" {
		return baseData{}, nil // Guest
	}
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(ctx, session.UserID)
	if err != nil {
		return baseData{}, fmt.Errorf(
			"fetching number of unread chats with unread messages: %w", err,
		)
	}
	return baseData{
		UnreadChats: unreadChats,
	}, nil
}

func (b Base) OnMessagingSent(
	sse *datastar.ServerSentEventGenerator,
	event EventMessagingSent,
	session SessionJWT,
) error {
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(sse.Context(), session.UserID)
	if err != nil {
		return err
	}

	return sse.PatchElementTempl(fragmentMessagesLink(unreadChats))
}

// Page404 is /not-found
type Page404 struct {
	App *App
	Base
}

func (p Page404) GET(
	r *http.Request,
	session SessionJWT,
) (body templ.Component, err error) {
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}
	return page404(session, baseData), nil
}

// Page500 is /whoops
type Page500 struct{ App *App }

func (Page500) GET(r *http.Request) (body templ.Component, err error) {
	return page500(), nil
}
