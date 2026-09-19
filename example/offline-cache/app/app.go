package app

import (
	"context"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// Session is the per-user authentication session. It carries no application
// data of its own; session.UserID() holds the user name.
type Session = datapages.Session[struct{}]

// App holds the application's dependencies.
type App struct {
	repo *domain.Repository
}

// NewApp creates the application backed by the given repository.
func NewApp(repo *domain.Repository) *App {
	return &App{repo: repo}
}

// SearchParams carries the live-search term. It is used both as GET query
// parameters (deep-linkable via ?q=) and as Datastar signals. The reflectsignal
// tag keeps the browser URL in sync as the "q" signal changes.
type SearchParams struct {
	Term string `json:"q" query:"q" reflectsignal:"q"`
}

// Head adds shared <head> content to every page.
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
	// Signing out drops the offline cache so this browser's signed-in pages are
	// not served offline to the next (guest) visitor. "/" re-bakes on the
	// redirect below; other pages re-cache as they are visited.
	pageCache.ClearAll()
	return true, datapages.Redirect{URL: href.PageIndex(href.QueryPageIndex{})}, nil
}

// Base is embedded into pages that render the shared navbar. It carries the
// data needed to render the top navigation for the current session.
type Base struct{ App *App }

type baseData struct {
	UserName      string
	UserAvatarURL string
	TicketCount   int
}

func (b Base) baseData(ctx context.Context, session Session) (baseData, error) {
	if session.IsGuest() {
		return baseData{}, nil // Guest
	}
	user, err := b.App.repo.UserByName(ctx, session.UserID())
	if err != nil {
		return baseData{}, err
	}
	tickets, err := b.App.repo.TicketsByUser(ctx, session.UserID())
	if err != nil {
		return baseData{}, err
	}
	return baseData{
		UserName:      user.Name,
		UserAvatarURL: user.AvatarImageURL,
		TicketCount:   len(tickets),
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

func (PageError500) GET(r *http.Request) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	return pageError500(), true, nil
}
