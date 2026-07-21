package app

import (
	"context"
	"hash/fnv"
	"io"
	"strconv"

	"github.com/a-h/templ"

	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// offlineCacheVersion derives a stable, non-zero version for an offline snapshot
// from the current session plus a content key. A snapshot is re-cached whenever
// the session (login/logout, different user) or its content changes. Callers
// compare it with != rather than <, since session identity has no ordering.
func offlineCacheVersion(session Session, contentKey string) uint64 {
	h := fnv.New64a()
	_, _ = io.WriteString(h, session.UserID)
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, contentKey)
	if v := h.Sum64(); v != 0 {
		return v
	}
	return 1 // Version() returns 0 when nothing is cached; never collide with it.
}

// ticketsOfflineVersion versions the "/tickets" snapshot by session and ticket
// count, so buying a ticket (or switching user) invalidates the cached copy.
func ticketsOfflineVersion(session Session, tickets []domain.Ticket) uint64 {
	return offlineCacheVersion(session, strconv.Itoa(len(tickets)))
}

// showOfflineVersion versions a "/shows/{slug}/" snapshot by session and whether
// the user owns a ticket, so purchasing flips its cached call-to-action.
func showOfflineVersion(session Session, hasTicket bool) uint64 {
	return offlineCacheVersion(session, strconv.FormatBool(hasTicket))
}

// reconnectScript reloads the page as soon as the browser regains connectivity,
// so a cached offline shell swaps back to the live page on its own.
const reconnectScript = `<script>` +
	`window.addEventListener('online',function(){location.reload()});` +
	`setInterval(function(){if(navigator.onLine){location.reload()}},5000);` +
	`</script>`

// indexOffline is the offline snapshot of the shows page ("/"): the same shell
// without the live search, which needs the server.
func indexOffline(session Session, base baseData) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := fragmentNavbar(session, base).Render(ctx, w); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div id="content"><div id="page-shows">`+
			`<header class="shows-header"><h1>Live shows &amp; events</h1>`+
			`<p class="subtitle">Internet connection lost, search is unavailable offline.</p>`+
			`</header>`+
			`<div id="show-results"><p class="empty">`+
			`Reconnecting as soon as you are back online.</p></div>`+
			`</div></div>`); err != nil {
			return err
		}
		if err := fragmentFooter().Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, reconnectScript)
		return err
	})
}

// loginOffline is the offline snapshot of the login page: the sign-in card says
// that signing in needs a connection.
func loginOffline() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div id="content"><div id="page-login">`+
			`<div class="card"><header><h1>Sign in</h1></header>`+
			`<section><p class="empty">Internet connection lost, please try again later.</p></section>`+
			`</div></div></div>`+reconnectScript)
		return err
	})
}
