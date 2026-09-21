package app

import (
	"context"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// Session identifies the signed-in user. It has no application data.
type Session = datapages.Session[struct{}]

// App provides ticketing handlers backed by a repository.
type App struct {
	repo *domain.Repository
}

// NewApp returns an application backed by repo.
func NewApp(repo *domain.Repository) *App {
	return &App{repo: repo}
}

// SearchParams defines the live-search query parameter and signal.
type SearchParams struct {
	Term string `json:"q" query:"q" reflectsignal:"q"`
}

// Head returns the shared head content.
func (*App) Head(r *http.Request) datapages.Head {
	return head()
}

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	// Prevent signed-in snapshots from being served after the session closes.
	// The destination and later visits repopulate the cache.
	pageCache.ClearAll()
	return true, datapages.Redirect{URL: href.PageIndex(href.QueryPageIndex{})}, nil
}

// Base provides navigation data to pages that embed it.
type Base struct{ App *App }

type baseData struct {
	UserName      string
	UserAvatarURL string
}

func (b Base) baseData(ctx context.Context, session Session) (baseData, error) {
	if session.IsGuest() {
		return baseData{}, nil
	}
	user, err := b.App.repo.UserByName(ctx, session.UserID())
	if err != nil {
		return baseData{}, err
	}
	return baseData{
		UserName:      user.Name,
		UserAvatarURL: user.AvatarImageURL,
	}, nil
}

// PageError404 is /not-found
type PageError404 struct {
	App *App
	Base
}

func (p PageError404) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}
	return pageError404(session, baseData), nil
}

// PageError500 is /whoops
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return pageError500(), nil
}
