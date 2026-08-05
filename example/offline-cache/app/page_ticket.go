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
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/httperr"
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
	path struct {
		Slug string `path:"nameslug"`
	},
) (body templ.Component, redirect string, err error) {
	if session.UserID == "" {
		return nil, href.PageLogin(href.QueryPageLogin{
			Next: href.PageTicket(path.Slug),
		}), nil
	}

	ticket, ok, err := p.App.repo.TicketForShow(r.Context(), session.UserID, path.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrShowNotFound) {
			return nil, "", httperr.NotFound
		}
		return nil, "", err
	}
	if !ok {
		// No ticket yet — send the user to the purchase page.
		return nil, href.PagePurchase(path.Slug), nil
	}

	qr, err := qrDataURI(ticket.Code)
	if err != nil {
		return nil, "", err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, "", err
	}

	view := pageTicket(session, ticket, qr, baseData)
	// Keep this ticket viewable offline, versioned by session and purchase time so
	// it is only re-cached when the client's copy is out of date.
	ver := offlineCacheVersion(session, strconv.FormatInt(ticket.PurchasedAt.Unix(), 10))
	if pageCache.Version() != ver {
		pageCache.Set(href.PageTicket(path.Slug), view, ver)
	}
	return view, "", nil
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
