package app

import (
	"context"
	"hash/fnv"
	"io"
	"strconv"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// offlineCacheVersion derives a stable, non-zero version for an offline snapshot
// from the current session plus a content key. A snapshot is re-cached whenever
// the session (login/logout, different user) or its content changes. Callers
// compare it with != rather than <, since session identity has no ordering.
func offlineCacheVersion(session Session, contentKey string) uint64 {
	h := fnv.New64a()
	_, _ = io.WriteString(h, session.UserID())
	_, _ = io.WriteString(h, "\x00")
	_, _ = io.WriteString(h, contentKey)
	if v := h.Sum64(); v != 0 {
		return v
	}
	// Version returns 0 for a cache miss, so snapshot versions start at 1.
	return 1
}

// ticketsOfflineVersion changes when the user or ticket count changes.
func ticketsOfflineVersion(session Session, tickets []domain.Ticket) uint64 {
	return offlineCacheVersion(session, strconv.Itoa(len(tickets)))
}

// showOfflineVersion changes when the user or ticket ownership changes.
func showOfflineVersion(session Session, hasTicket bool) uint64 {
	return offlineCacheVersion(session, strconv.FormatBool(hasTicket))
}

// reconnectScript reloads the cached page when the browser regains a network
// connection.
const reconnectScript = `<script>` +
	`window.addEventListener('online',function(){location.reload()});` +
	`setInterval(function(){if(navigator.onLine){location.reload()}},5000);` +
	`</script>`

// indexOffline is the shows page snapshot without server-dependent search.
func indexOffline(session Session, base baseData) datapages.Component {
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

// loginOffline reports that sign-in requires a network connection.
func loginOffline() datapages.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div id="content"><div id="page-login">`+
			`<div class="card"><header><h1>Sign in</h1></header>`+
			`<section><p class="empty">Internet connection lost, please try again later.</p></section>`+
			`</div></div></div>`+reconnectScript)
		return err
	})
}
