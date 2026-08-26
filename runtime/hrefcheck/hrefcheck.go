// Package hrefcheck reports which URLs may go into an href attribute and
// builds the URL path of a static asset.
//
// Application code must not import this package.
package hrefcheck

import (
	"path"
	"strings"
)

// AssetPath is the URL path of the static asset p under prefix.
//
// Concatenation is the fast path, taken for the clean relative names a template writes.
// Anything else goes through path.Join, which normalizes it.
// Join does not confine the result to prefix. AssetPath("/static/", "../x") is "/x".
// The result is a URL the router resolves, not a file system path.
// It reaches the asset handler only while it keeps the prefix.
func AssetPath(prefix, p string) string {
	if p == "" || p == "." || p[0] == '/' ||
		strings.HasPrefix(p, "..") || p != path.Clean(p) {
		return path.Join(prefix, p)
	}
	return prefix + p
}

// templURLSchemes are the schemes the templ sanitizer keeps.
// It rewrites every other scheme to "about:invalid#TemplFailedSanitizationURL",
// which is what a URL written through an attribute expression goes through.
// A literal attribute is written as it stands and reaches no sanitizer.
var templURLSchemes = []string{"http", "https", "mailto", "tel", "ftp", "ftps"}

// IsRenderedAsWritten reports whether s survives the templ sanitizer,
// which every URL written as an attribute expression passes through.
// A URL it drops renders as "about:invalid#TemplFailedSanitizationURL" and links nowhere.
func IsRenderedAsWritten(s string) bool {
	scheme, ok := scheme(strings.TrimSpace(s))
	if !ok {
		// No scheme of its own: a fragment, a path or a protocol-relative URL,
		// all of which the sanitizer keeps.
		return true
	}
	for _, allowed := range templURLSchemes {
		if strings.EqualFold(scheme, allowed) {
			return true
		}
	}
	return false
}

// IsAllowedNonRelativeHref returns false for:
//   - empty/whitespace
//   - query-only URLs like ?tab=settings
//   - internal/root-relative paths like /login
//   - relative paths like ./x, ../x, x, foo/bar
//   - javascript: URLs
//
// It returns true for:
//   - fragment-only hrefs like #section
//   - protocol-relative URLs like //cdn.example.com
//   - absolute/schemed URLs like https:, mailto:, tel:, sms:, ftp:, data:
//
// It reports what belongs in an href, not what a browser receives:
// a URL written as an attribute expression also passes the templ sanitizer,
// which keeps fewer schemes. See [IsRenderedAsWritten].
//
// Limitation: cannot detect absolute links to the same domain
// (e.g. https://mydomain.com/login).
func IsAllowedNonRelativeHref(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// Allow fragment-only links.
	if strings.HasPrefix(s, "#") {
		return true
	}

	// Disallow query-only/current-path navigations.
	if strings.HasPrefix(s, "?") {
		return false
	}

	// Allow protocol-relative URLs (//cdn.example.com).
	if strings.HasPrefix(s, "//") {
		return true
	}

	// Disallow obvious internal paths.
	if strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") {
		return false
	}

	// If it has an explicit URI scheme, allow it unless banned.
	if scheme, ok := scheme(s); ok {
		return !strings.EqualFold(scheme, "javascript")
	}

	// Everything else is a plain relative path.
	return false
}

// scheme returns the RFC 3986 scheme s starts with.
func scheme(s string) (string, bool) {
	for i := range len(s) {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return "", false
			}
		case c == ':':
			if i == 0 {
				return "", false
			}
			return s[:i], true
		default:
			return "", false
		}
	}
	return "", false
}
