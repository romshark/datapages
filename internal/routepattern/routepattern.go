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

// EndsInWildcard reports whether the route's last segment is a {name...} wildcard,
// which matches the rest of the path.
func EndsInWildcard(route string) bool {
	last := route[strings.LastIndex(route, "/")+1:]
	return strings.HasPrefix(last, "{") && strings.HasSuffix(last, "...}")
}

// WithTrailingSlash strips any {$} suffix and ensures a trailing slash.
//   - "/settings" -> "/settings/"
//   - "/user/{name}/{$}" -> "/user/{name}/"
//   - "/" -> "/"
func WithTrailingSlash(route string) string {
	route = strings.TrimSuffix(route, "{$}")
	if !strings.HasSuffix(route, "/") {
		return route + "/"
	}
	return route
}

// Segments splits a route into alternating literal and variable segments:
//
//	URL = literals[0] + vars[0] + literals[1] + ... + literals[len(literals)-1]
//
// The last literal carries the trailing slash.
func Segments(route string) (literals []string, vars []string) {
	r := strings.TrimSuffix(route, "{$}")
	r = strings.TrimSuffix(r, "/")

	for {
		i := strings.IndexByte(r, '{')
		if i < 0 {
			if r == "" {
				literals = append(literals, "/")
			} else {
				literals = append(literals, r+"/")
			}
			return literals, vars
		}
		literals = append(literals, r[:i])
		r = r[i+1:]

		j := strings.IndexByte(r, '}')
		if j < 0 {
			return literals, vars
		}
		name := strings.TrimSuffix(r[:j], "...")
		if name != "$" && name != "" {
			vars = append(vars, name)
		}
		r = r[j+1:]
	}
}
