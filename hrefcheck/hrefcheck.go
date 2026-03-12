package hrefcheck

import (
	"net/url"
	"strings"
)

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
	u, err := url.Parse(s)
	if err == nil && u.Scheme != "" {
		if strings.EqualFold(u.Scheme, "javascript") {
			return false
		}
		return true
	}

	// Everything else is a plain relative path.
	return false
}
