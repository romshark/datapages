// Package app demonstrates instant page loads backed by the service worker.
//
// Two pages cache a shim of themselves: the page chrome with placeholders in
// place of the slow part. Each later visit paints the shim from cache at once.
// The worker then fetches the live page and Datastar morphs it in. The other
// pages cache nothing and wait for the server on every visit. No page carries
// Datastar attributes; Datapages emits the shim's trigger.
package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/fast-shim/datapagesgen/href"
)

// SlowQuery is how long the "database" takes. It is what a shim hides.
const SlowQuery = 900 * time.Millisecond

// shimVersion versions the cached shims. They hold no data. Only a code change
// bumps it.
const shimVersion = 1

type App struct{}

func (*App) Head(r *http.Request) templ.Component {
	return raw(`<title>Fast Shim</title>` +
		`<meta name="viewport" content="width=device-width,initial-scale=1"/>` +
		// Inline icon: without one the browser requests /favicon.ico and 404s.
		`<link rel="icon" href="data:image/svg+xml,` +
		`%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E` +
		`%3Crect width='16' height='16' rx='4' fill='%239db4ff'/%3E%3C/svg%3E"/>` +
		`<style>` +
		`body{font:16px/1.5 system-ui,sans-serif;margin:0;background:#0f1020;color:#e8e8f0}` +
		`main{max-width:40rem;margin:0 auto;padding:2rem 1.25rem}` +
		`nav{display:flex;gap:1rem;padding:1rem 1.25rem;background:#181a30}` +
		`nav a{color:#9db4ff;text-decoration:none}` +
		`h1{margin:0 0 .25rem}` +
		`.muted{color:#9aa0b5}` +
		`.row{padding:.7rem .9rem;background:#181a30;border-radius:.5rem;margin:.4rem 0}` +
		`.skeleton{background:linear-gradient(90deg,#181a30,#242848,#181a30);` +
		`background-size:200% 100%;animation:sh 1.1s infinite;color:transparent;border-radius:.5rem}` +
		`@keyframes sh{0%{background-position:200% 0}100%{background-position:-200% 0}}` +
		`</style>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body templ.Component, err error) {
	rows, err := slowRows(r.Context(), "Index")
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageIndex(), shim(titleIndex, len(rows)), shimVersion)
	}
	return page(titleIndex, rows, true), nil
}

// PageSubpage is /subpage
type PageSubpage struct{ App *App }

func (p PageSubpage) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body templ.Component, err error) {
	rows, err := slowRows(r.Context(), "Subpage")
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageSubpage(), shim(titleSubpage, len(rows)), shimVersion)
	}
	return page(titleSubpage, rows, true), nil
}

// PageNoShim is /noshim
//
// Caches nothing. Every visit waits for the server. The control the shim pages
// are compared against.
type PageNoShim struct{ App *App }

func (p PageNoShim) GET(r *http.Request) (body templ.Component, err error) {
	rows, err := slowRows(r.Context(), "No Shim")
	if err != nil {
		return nil, err
	}
	return page(titleNoShim, rows, false), nil
}

// PageNoShim2 is /noshim2
//
// A second uncached page. Navigating between the two shows the wait on every
// hop, not only on reload.
type PageNoShim2 struct{ App *App }

func (p PageNoShim2) GET(r *http.Request) (body templ.Component, err error) {
	rows, err := slowRows(r.Context(), "No Shim 2")
	if err != nil {
		return nil, err
	}
	return page(titleNoShim2, rows, false), nil
}

const (
	titleIndex   = "Shim Index"
	titleSubpage = "Shim Subpage"
	titleNoShim  = "No Shim"
	titleNoShim2 = "No Shim 2"
)

// slowRows stands in for a slow backend query.
func slowRows(ctx context.Context, prefix string) ([]string, error) {
	select {
	case <-time.After(SlowQuery):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	rows := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		rows = append(rows, fmt.Sprintf("%s entry %d", prefix, i))
	}
	return rows, nil
}

func page(title string, rows []string, shimmed bool) templ.Component {
	return doc(title, func(w io.Writer) {
		for _, r := range rows {
			fmt.Fprintf(w, `<div class="row">%s</div>`, templ.EscapeString(r))
		}
		if shimmed {
			fmt.Fprintf(w, `<p class="muted">Live contents, morphed in after %s. `+
				`Reload to see the shim first.</p>`, SlowQuery)
			return
		}
		fmt.Fprintf(w, `<p class="muted">No shim cached, so this page blocks for %s `+
			`on every visit.</p>`, SlowQuery)
	})
}

// shim is the same page with placeholder rows instead of data.
func shim(title string, rows int) templ.Component {
	return doc(title, func(w io.Writer) {
		for range rows {
			_, _ = io.WriteString(w, `<div class="row skeleton">&nbsp;</div>`)
		}
		_, _ = io.WriteString(w, `<p class="muted">Served from cache, loading…</p>`)
	})
}

func doc(title string, content func(io.Writer)) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, _ = fmt.Fprintf(w,
			`<nav><a href="%s">%s</a><a href="%s">%s</a>`+
				`<a href="%s">%s</a><a href="%s">%s</a></nav>`,
			href.PageIndex(), titleIndex,
			href.PageSubpage(), titleSubpage,
			href.PageNoShim(), titleNoShim,
			href.PageNoShim2(), titleNoShim2)
		_, _ = fmt.Fprintf(w, `<main><h1>%s</h1>`, templ.EscapeString(title))
		content(w)
		_, _ = io.WriteString(w, `</main>`)
		return nil
	})
}

func raw(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}
