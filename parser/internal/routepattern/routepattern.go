// Package routepattern parses net/http ServeMux route patterns.
package routepattern

import (
	"iter"
	"strings"
)

// Vars returns an iterator over the wildcard variable names in a route pattern
// like /foo/{id}/bar/{slug}. It skips the special {$} exact-match marker
// and strips the {name...} trailing wildcard suffix.
func Vars(route string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			i := strings.IndexByte(route, '{')
			if i < 0 {
				return
			}
			route = route[i+1:]
			j := strings.IndexByte(route, '}')
			if j < 0 {
				return
			}
			name := route[:j]
			route = route[j+1:]
			name = strings.TrimSuffix(name, "...")
			if name != "$" && name != "" {
				if !yield(name) {
					return
				}
			}
		}
	}
}
