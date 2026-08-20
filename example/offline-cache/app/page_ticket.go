package app

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
)

// PageTicket is /shows/{nameslug}/ticket
type PageTicket struct {
	App *App
	Base
}

func (p PageTicket) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		Slug string `path:"nameslug"`
	}],
) (body datapages.Component, redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		return nil, datapages.Redirect{URL: href.PageLogin(href.QueryPageLogin{
			Next: href.PageTicket(path.Values.Slug),
		})}, nil
	}

	ticket, ok, err := p.App.repo.TicketForShow(r.Context(), session.UserID(), path.Values.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrShowNotFound) {
			return nil, datapages.Redirect{}, datapages.ErrNotFound
		}
		return nil, datapages.Redirect{}, err
	}
	if !ok {
		// No ticket yet — send the user to the purchase page.
		return nil, datapages.Redirect{URL: href.PagePurchase(path.Values.Slug)}, nil
	}

	qr, err := qrDataURI(ticket.Code)
	if err != nil {
		return nil, datapages.Redirect{}, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, datapages.Redirect{}, err
	}

	view := pageTicket(session, ticket, qr, baseData)
	// Keep this ticket viewable offline, versioned by session and purchase time so
	// it is only re-cached when the client's copy is out of date.
	ver := offlineCacheVersion(session, strconv.FormatInt(ticket.PurchasedAt.Unix(), 10))
	if pageCache.Version() != ver {
		pageCache.Set(href.PageTicket(path.Values.Slug), view, ver)
	}
	return view, datapages.Redirect{}, nil
}

// qrDataURI renders content as a PNG QR code and returns it as a data: URI
// suitable for use as an <img src> value.
func qrDataURI(content string) (templ.SafeURL, error) {
	png, err := qrcode.Encode(content, qrcode.Medium, 320)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(png)
	return templ.SafeURL("data:image/png;base64," + encoded), nil
}
