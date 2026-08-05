package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
)

// PageTickets is /tickets
type PageTickets struct {
	App *App
	Base
}

func (p PageTickets) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
) (
	body templ.Component,
	redirect string,
	err error,
) {
	if session.UserID == "" {
		return nil, href.PageLogin(href.QueryPageLogin{
			Next: href.PageTickets(),
		}), nil
	}

	tickets, err := p.App.repo.TicketsByUser(r.Context(), session.UserID)
	if err != nil {
		return nil, "", err
	}
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, "", err
	}

	view := pageTickets(session, tickets, baseData)
	// Cache the tickets list with the tickets so it is viewable offline. Versioned
	// by session and ticket count so a purchase or a different user refreshes it.
	if ver := ticketsOfflineVersion(session, tickets); pageCache.Version() != ver {
		pageCache.Set(href.PageTickets(), view, ver)
	}
	return view, "", nil
}
