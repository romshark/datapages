package app

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/httperr"
)

// PageShow is /shows/{nameslug}
type PageShow struct {
	App *App
	Base
}

func (p PageShow) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	path struct {
		Slug string `path:"nameslug"`
	},
) (body templ.Component, head templ.Component, err error) {
	show, err := p.App.repo.ShowBySlug(r.Context(), path.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrShowNotFound) {
			return nil, nil, httperr.NotFound
		}
		return nil, nil, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, nil, err
	}

	// Determine whether the current user already owns a ticket for this show.
	hasTicket := false
	if session.UserID != "" {
		_, ok, err := p.App.repo.TicketForShow(r.Context(), session.UserID, show.Slug)
		if err != nil {
			return nil, nil, err
		}
		hasTicket = ok
	}

	view := pageShow(session, show, hasTicket, baseData)
	// Cache this show lazily so it stays viewable offline once visited. Versioned
	// by session and ticket ownership so login/logout or a purchase refreshes it.
	if ver := showOfflineVersion(session, hasTicket); pageCache.Version() != ver {
		pageCache.Set(href.PageShow(show.Slug), view, ver)
	}
	head = headShow(show)
	return view, head, nil
}
