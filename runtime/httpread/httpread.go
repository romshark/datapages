// Package httpread reads a cookie or a query parameter off a request.
// It provides optimized versions of equivalent functions from net/http and net/url.
//
// Application code must not import this package.
package httpread

import (
	"net/http"
	"net/url"
	"strings"
)

// asciiSpace is what net/textproto trims off a header value.
const asciiSpace = " \t\n\r"

// maxScannedCookies is the number of cookies CookieValue reads itself.
// Above it net/http decides, which is where its own limit lives.
const maxScannedCookies = 64

// IsCookieName reports whether s is a name a cookie may carry.
func IsCookieName(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// CookieValue returns the value of the named cookie of r.
// It reads the Cookie header the way [net/http.Request.Cookie] reads it:
// a pair whose value carries a byte no cookie value may carry is skipped.
func CookieValue(r *http.Request, name string) (value string, ok bool) {
	if !IsCookieName(name) {
		return "", false
	}
	lines := r.Header["Cookie"]
	pairs := 0
	for _, line := range lines {
		pairs += strings.Count(line, ";") + 1
	}
	if pairs > maxScannedCookies {
		c, err := r.Cookie(name)
		if err != nil {
			return "", false
		}
		return c.Value, true
	}
	for _, line := range lines {
		for line != "" {
			var pair string
			pair, line, _ = strings.Cut(line, ";")
			pair = strings.Trim(pair, asciiSpace)
			if pair == "" {
				continue
			}
			n, v, _ := strings.Cut(pair, "=")
			if strings.Trim(n, asciiSpace) != name {
				continue
			}
			if len(v) > 1 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			valid := true
			for i := range len(v) {
				if b := v[i]; b < 0x20 || b >= 0x7f ||
					b == '"' || b == ';' || b == '\\' {
					valid = false
					break
				}
			}
			if valid {
				return v, true
			}
		}
	}
	return "", false
}

// queryLookup returns the first value of key in rawQuery.
// It reads what [net/url.URL.Query] parses: pairs are separated by "&",
// a pair carrying ";" is skipped, and both sides are query-unescaped.
func queryLookup(rawQuery, key string) (value string, ok bool) {
	for rawQuery != "" {
		var pair string
		pair, rawQuery, _ = strings.Cut(rawQuery, "&")
		if pair == "" || strings.Contains(pair, ";") {
			continue
		}
		name, value, _ := strings.Cut(pair, "=")
		name, err := url.QueryUnescape(name)
		if err != nil || name != key {
			continue
		}
		value, err = url.QueryUnescape(value)
		if err != nil {
			continue
		}
		return value, true
	}
	return "", false
}

// QueryValue returns the first value of key in rawQuery,
// which is what [net/url.Values.Get] returns for it.
func QueryValue(rawQuery, key string) string {
	v, _ := queryLookup(rawQuery, key)
	return v
}

// QueryHas reports whether rawQuery carries key,
// which is what [net/url.Values.Has] reports for it.
func QueryHas(rawQuery, key string) bool {
	_, ok := queryLookup(rawQuery, key)
	return ok
}
