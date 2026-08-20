package hrefcheck

import "strings"

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
