package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
)

// PagePurchase is /shows/{nameslug}/purchase
type PagePurchase struct {
	App *App
	Base
}

func (p PagePurchase) GET(
	r *http.Request,
	session Session,
	path struct {
		Slug string `path:"nameslug"`
	},
) (body datapages.Component, redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		// Guests must sign in before purchasing.
		return nil, datapages.Redirect{URL: href.PageLogin(href.QueryPageLogin{
			Next: href.PagePurchase(path.Slug),
		})}, nil
	}

	show, err := p.App.repo.ShowBySlug(r.Context(), path.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrShowNotFound) {
			return nil, datapages.Redirect{}, datapages.ErrNotFound
		}
		return nil, datapages.Redirect{}, err
	}

	// If the user already owns a ticket, jump straight to it.
	if _, ok, err := p.App.repo.TicketForShow(
		r.Context(), session.UserID(), show.Slug,
	); err != nil {
		return nil, datapages.Redirect{}, err
	} else if ok {
		return nil, datapages.Redirect{URL: href.PageTicket(show.Slug)}, nil
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, datapages.Redirect{}, err
	}
	return pagePurchase(session, show, baseData), datapages.Redirect{}, nil
}

// POSTConfirm is /shows/{nameslug}/purchase/confirm
func (p PagePurchase) POSTConfirm(
	r *http.Request,
	sse datapages.SSE,
	pageCache datapages.PageCacheWriter,
	session Session,
	path struct {
		Slug string `path:"nameslug"`
	},
) error {
	if session.IsGuest() {
		return navigate(sse, href.PageLogin(href.QueryPageLogin{
			Next: href.PagePurchase(path.Slug),
		}))
	}

	_, err := p.App.repo.BuyTicket(r.Context(), session.UserID(), path.Slug)
	switch {
	case err == nil, errors.Is(err, domain.ErrTicketExists):
		// Refresh the offline cache so the new ticket and the updated tickets
		// list are viewable offline right away, then navigate to the ticket.
		if err := p.refreshOfflineCache(r, pageCache, session, path.Slug); err != nil {
			return err
		}
		return navigate(sse, href.PageTicket(path.Slug))
	case errors.Is(err, domain.ErrShowNotFound):
		return datapages.ErrNotFound
	case errors.Is(err, domain.ErrShowSoldOut):
		return datapages.ErrBadRequest
	default:
		return err
	}
}

// refreshOfflineCache re-caches the tickets list and the just-purchased ticket
// so both are available offline immediately after a purchase.
func (p PagePurchase) refreshOfflineCache(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	session Session,
	slug string,
) error {
	ctx := r.Context()
	baseData, err := p.baseData(ctx, session)
	if err != nil {
		return err
	}

	tickets, err := p.App.repo.TicketsByUser(ctx, session.UserID())
	if err != nil {
		return err
	}
	pageCache.Set(
		href.PageTickets(),
		pageTickets(session, tickets, baseData),
		ticketsOfflineVersion(session, tickets),
	)

	ticket, ok, err := p.App.repo.TicketForShow(ctx, session.UserID(), slug)
	if err != nil {
		return err
	}
	if ok {
		qr, err := qrDataURI(ticket.Code)
		if err != nil {
			return err
		}
		pageCache.Set(
			href.PageTicket(slug),
			pageTicket(session, ticket, qr, baseData),
			offlineCacheVersion(session, strconv.FormatInt(ticket.PurchasedAt.Unix(), 10)),
		)
	}

	// Refresh the show page too so its offline call-to-action flips from "Buy" to
	// "View your ticket".
	if show, err := p.App.repo.ShowBySlug(ctx, slug); err == nil {
		pageCache.Set(
			href.PageShow(slug),
			pageShow(session, show, true, baseData),
			showOfflineVersion(session, true),
		)
	}
	return nil
}

// navigate performs a client-side redirect over the SSE stream.
func navigate(sse datapages.SSE, url string) error {
	return sse.ExecuteScript(fmt.Sprintf("window.location=%q", url))
}
