// Package urlpath provides shared URL path utilities.
package urlpath

import "strings"

// Clean trims trailing slashes from p, preserving "/" as-is.
func Clean(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}
