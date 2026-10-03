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
	ErrAssetsURLPrefixRoot = errors.New(
		"Assets.URLPrefix must not be \"/\"; it would conflict with page routes",
	)
	ErrAssetsURLPrefixInvalidChar = errors.New(
		"Assets.URLPrefix contains invalid characters; " +
			"use only ASCII letters, digits, '-', '.', '_', '~' and slashes",
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
	// The prefix starts and ends with a slash, which puts every segment
	// between two of them.
	if strings.Contains(s, "/./") || strings.Contains(s, "/../") {
		return ErrAssetsURLPrefixDotSegment
	}
	if strings.Contains(s, `\`) {
		return ErrAssetsURLPrefixBackslash
	}
	for i := range len(s) {
		if !isURLPrefixChar(s[i]) {
			return ErrAssetsURLPrefixInvalidChar
		}
	}
	return nil
}

// isURLPrefixChar reports whether c may appear in an assets URL prefix:
// a letter, a digit, '-', '.', '_', '~' or '/'. None of these needs percent-encoding.
// The server compares the prefix with the decoded request path,
// which an encoded prefix such as /my%20files/ never matches.
func isURLPrefixChar(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~' || c == '/'
}
