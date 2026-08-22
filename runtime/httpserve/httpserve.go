package httpserve

import (
	"fmt"
	"io"
	"net/http"

	"github.com/romshark/datapages"
)

// IsDatastarRequest reports whether r was issued by the Datastar client.
func IsDatastarRequest(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
}

// Redirect writes the redirect to w and reports whether it wrote one.
// A Datastar request cannot follow an HTTP redirect, which is why one
// navigates client-side instead.
func Redirect(
	w http.ResponseWriter, r *http.Request, redirect datapages.Redirect,
) (exit bool) {
	if redirect.URL == "" {
		return false
	}

	if IsDatastarRequest(r) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = fmt.Fprintf(w, "window.location = %q;", redirect.URL)
		return true
	}

	status := redirect.Status
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		// OK
	default:
		status = http.StatusFound
	}

	http.Redirect(w, r, redirect.URL, status)
	return true
}

// DevNoCache stops the browser from caching what next serves.
func DevNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// WriteReloadOnVisibility writes the body attribute that reloads a page
// the browser shows again after the server restarted.
func WriteReloadOnVisibility(w io.Writer) {
	_, _ = io.WriteString(w,
		`data-on:visibilitychange__window="`+
			`if (!document.hidden) window.location.reload()" `)
}
