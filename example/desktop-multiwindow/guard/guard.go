// Package guard lets only this app's windows reach a server on loopback,
// which any process on the machine can connect to.
//
// Each launch makes a new token and passes it to the windows in their URL.
// The first request swaps the token for a cookie, which every port of 127.0.0.1 shares.
// Requests without the token get 403. A process running as the same user can
// still read the token from the app's memory or the webview's cookies.
package guard

import (
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/url"
)

// name is the name of the token's query parameter and cookie.
const name = "launch-token"

// URL returns the URL a window opens to load the page from addr.
func URL(addr, token string) string {
	return "http://" + addr + "/?" + url.Values{name: {token}}.Encode()
}

// Handler passes a request to next if it carries token and its Host is the
// address of the listener that accepted it.
//
// The Host check blocks DNS rebinding, where a page of
// another site points its own name at 127.0.0.1.
func Handler(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
		if local == nil || r.Host != local.String() {
			reject(w, r, "host is not the listener address")
			return
		}
		q := r.URL.Query()
		if t := q.Get(name); t != "" {
			if !equal(t, token) {
				reject(w, r, "wrong token in the URL")
				return
			}
			// Strict keeps the webview from sending the cookie with requests
			// from other sites.
			http.SetCookie(w, &http.Cookie{
				Name:     name,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			q.Del(name)
			u := *r.URL
			u.RawQuery = q.Encode()
			// The redirect removes the token from the window's URL,
			// so a reload or a copied link doesn't carry it.
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(name)
		if err != nil || !equal(c.Value, token) {
			reject(w, r, "no token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func reject(w http.ResponseWriter, r *http.Request, reason string) {
	slog.Warn("rejected request",
		slog.String("reason", reason),
		slog.String("host", r.Host),
		slog.String("path", r.URL.Path),
		slog.String("remote", r.RemoteAddr))
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

// equal compares in constant time.
// Otherwise the time of a rejection would reveal how much of a guess was right.
func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
