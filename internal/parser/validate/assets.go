package validate

import (
	"errors"
	"strings"
)

// Sentinel errors for assets validation.
var (
	ErrAssetsDirRequired = errors.New(
		"Assets.Dir is required when Assets is set",
	)
	ErrAssetsURLPrefixRequired = errors.New(
		"Assets.URLPrefix is required when Assets is set",
	)
	ErrAssetsURLPrefixNoLeadingSlash = errors.New(
		"Assets.URLPrefix must start with '/'",
	)
	ErrAssetsURLPrefixNoTrailingSlash = errors.New(
		"Assets.URLPrefix must end with '/'",
	)
	ErrAssetsURLPrefixDoubleSlash = errors.New(
		"Assets.URLPrefix must not contain double slashes",
	)
	ErrAssetsURLPrefixQueryString = errors.New(
		"Assets.URLPrefix must not contain a query string",
	)
	ErrAssetsURLPrefixFragment = errors.New(
		"Assets.URLPrefix must not contain a fragment",
	)
	ErrAssetsURLPrefixDotSegment = errors.New(
		"Assets.URLPrefix must not contain dot segments",
	)
	ErrAssetsURLPrefixBackslash = errors.New(
		"Assets.URLPrefix must not contain backslashes",
	)
	ErrAssetsURLPrefixEncodedTraversal = errors.New(
		"Assets.URLPrefix must not contain percent-encoded dots, " +
			"slashes, or backslashes",
	)
	ErrAssetsURLPrefixRoot = errors.New(
		"Assets.URLPrefix must not be \"/\"; it would conflict with page routes",
	)
	ErrAssetsURLPrefixInvalidChar = errors.New(
		"Assets.URLPrefix contains invalid characters; " +
			"use only ASCII letters, digits, hyphens, underscores, and slashes",
	)
)

// AssetsURLPrefix checks that s is a valid URL path prefix for embedded files.
func AssetsURLPrefix(s string) error {
	if !strings.HasPrefix(s, "/") {
		return ErrAssetsURLPrefixNoLeadingSlash
	}
	if !strings.HasSuffix(s, "/") {
		return ErrAssetsURLPrefixNoTrailingSlash
	}
	if s == "/" {
		return ErrAssetsURLPrefixRoot
	}
	if strings.Contains(s, "//") {
		return ErrAssetsURLPrefixDoubleSlash
	}
	if strings.Contains(s, "?") {
		return ErrAssetsURLPrefixQueryString
	}
	if strings.Contains(s, "#") {
		return ErrAssetsURLPrefixFragment
	}
	if strings.Contains(s, "/.") {
		return ErrAssetsURLPrefixDotSegment
	}
	if strings.Contains(s, `\`) {
		return ErrAssetsURLPrefixBackslash
	}
	if i := strings.Index(s, "%"); i >= 0 {
		if err := checkPercentEncoding(s[i:]); err != nil {
			return err
		}
	}
	for i := range len(s) {
		c := s[i]
		if c <= ' ' || c >= 0x7f || c == '{' || c == '}' ||
			c == '<' || c == '>' || c == '|' || c == '^' || c == '`' {
			return ErrAssetsURLPrefixInvalidChar
		}
	}
	return nil
}

// checkPercentEncoding scans s (starting from the first '%') for percent-encoded
// sequences and rejects encoded dots (%2e/%2E), slashes (%2f/%2F), and
// backslashes (%5c/%5C) that could bypass path traversal checks.
func checkPercentEncoding(s string) error {
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+2 >= len(s) {
			return ErrAssetsURLPrefixInvalidChar
		}
		hi, lo := s[i+1], s[i+2]
		if !isHexDigit(hi) || !isHexDigit(lo) {
			return ErrAssetsURLPrefixInvalidChar
		}
		upper := strings.ToUpper(string([]byte{hi, lo}))
		if upper == "2E" || upper == "2F" || upper == "5C" {
			return ErrAssetsURLPrefixEncodedTraversal
		}
		i += 2
	}
	return nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
