// Package shared declares types outside the app package,
// which the parser does not read.
package shared

import "net/http"

type Likes struct{}

// POSTLike is /like
func (Likes) POSTLike(_ *http.Request) error { return nil }

type Cache[T any] struct{ Items map[string]T }
