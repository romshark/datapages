package routepattern

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVars tests the variable names read out of a route pattern,
// wildcards included. "{$}" is no variable, and a malformed pattern
// yields none rather than a partial name.
func TestVars(t *testing.T) {
	tests := map[string]struct {
		route string
		want  []string
	}{
		"root":           {"/", nil},
		"static":         {"/items", nil},
		"single var":     {"/items/{id}", []string{"id"}},
		"var mid-path":   {"/items/{id}/details", []string{"id"}},
		"two vars":       {"/users/{name}/posts/{slug}", []string{"name", "slug"}},
		"exact match":    {"/exact/{$}", nil},
		"var + exact":    {"/items/{id}/{$}", []string{"id"}},
		"wildcard":       {"/files/{path...}", []string{"path"}},
		"var + wildcard": {"/a/{x}/b/{y...}", []string{"x", "y"}},
		"empty braces":   {"/{}", nil},
		"empty string":   {"", nil},
		"unclosed brace": {"/items/{id", nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t, tt.want,
				slices.Collect(Vars(tt.route)),
			)
		})
	}
}

// TestVarsEarlyBreak tests a caller that stops after the first name,
// which is the iterator's early-return path.
func TestVarsEarlyBreak(t *testing.T) {
	// Stop consuming after the first variable to cover
	// the !yield(name) early-return branch.
	var got string
	for v := range Vars("/a/{x}/b/{y}") {
		got = v
		break
	}
	require.Equal(t, "x", got)
}

// TestEndsInWildcard tests the pattern shape that makes a route match a whole subtree.
// It has to be the last segment: a "{path...}" anywhere else is not one.
func TestEndsInWildcard(t *testing.T) {
	tests := map[string]struct {
		route string
		want  bool
	}{
		"root":              {"/", false},
		"static":            {"/items", false},
		"var":               {"/items/{id}", false},
		"wildcard":          {"/files/{path...}", true},
		"wildcard mid-path": {"/files/{path...}/x", false},
		"exact match":       {"/items/{$}", false},
		"empty string":      {"", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, EndsInWildcard(tt.route))
		})
	}
}

// TestWithTrailingSlash tests the canonical form every page route is registered under.
// "{$}" is dropped rather than kept, since the trailing slash already says what it said.
func TestWithTrailingSlash(t *testing.T) {
	tests := map[string]struct {
		route string
		want  string
	}{
		"root":         {"/", "/"},
		"root exact":   {"/{$}", "/"},
		"static":       {"/settings", "/settings/"},
		"static slash": {"/settings/", "/settings/"},
		"var":          {"/user/{name}", "/user/{name}/"},
		"var exact":    {"/user/{name}/{$}", "/user/{name}/"},
		"wildcard":     {"/files/{path...}", "/files/{path...}/"},
		"empty string": {"", "/"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, WithTrailingSlash(tt.route))
		})
	}
}

// TestSegments tests the split the generated href helpers are built from:
// the literal pieces they concatenate and the variable names that go between them.
// There is always one more literal than variable, which lets a helper write
// literal, value, literal.
func TestSegments(t *testing.T) {
	tests := map[string]struct {
		route        string
		wantLiterals []string
		wantVars     []string
	}{
		"root":         {"/", []string{"/"}, nil},
		"static":       {"/items", []string{"/items/"}, nil},
		"static slash": {"/items/", []string{"/items/"}, nil},
		"single var": {
			"/post/{slug}", []string{"/post/", "/"}, []string{"slug"},
		},
		"var mid-path": {
			"/post/{slug}/edit", []string{"/post/", "/edit/"}, []string{"slug"},
		},
		"two vars": {
			"/users/{name}/posts/{slug}",
			[]string{"/users/", "/posts/", "/"},
			[]string{"name", "slug"},
		},
		"exact match": {"/items/{$}", []string{"/items/"}, nil},
		"wildcard": {
			"/files/{path...}", []string{"/files/", "/"}, []string{"path"},
		},
		"empty braces":   {"/{}", []string{"//"}, nil},
		"unclosed brace": {"/items/{id", []string{"/items/"}, nil},
		"empty string":   {"", []string{"/"}, nil},
		// A nameless wildcard away from the end contributes no variable,
		// so it must not start a literal of its own: the callers reconstruct
		// the URL as literals[0] + vars[0] + ... and index params[i-1].
		"exact match not at end": {
			"/item/{id}/{$}/b", []string{"/item/", "//b/"}, []string{"id"},
		},
		"empty braces not at end": {
			"/item/{id}/{}/b", []string{"/item/", "//b/"}, []string{"id"},
		},
		"exact match first": {
			"/{$}/item/{id}", []string{"//item/", "/"}, []string{"id"},
		},
		"stray closing brace": {"/items/}id", []string{"/items/}id/"}, nil},
		"double opening brace": {
			"/items/{{id}", []string{"/items/", "/"}, []string{"{id"},
		},
		"double closing brace": {
			"/items/{id}}", []string{"/items/", "}/"}, []string{"id"},
		},
		"adjacent vars": {
			"/items/{a}{b}", []string{"/items/", "", "/"}, []string{"a", "b"},
		},
		"nameless wildcard":  {"/items/{...}", []string{"/items//"}, nil},
		"opening brace only": {"{", []string{""}, nil},
		"closing brace only": {"}", []string{"}/"}, nil},
		// The "{$}" suffix is trimmed before the scan,
		// which leaves an unterminated brace. [Vars] reads "{$" as a name here.
		"exact match marker inside a brace": {"{{$}", []string{""}, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			literals, vars := Segments(tt.route)
			require.Equal(t, tt.wantLiterals, literals)
			require.Equal(t, tt.wantVars, vars)
		})
	}
}

// FuzzSegments tests the invariant both generator writers rely on:
// [Segments] returns one literal more than it returns variables, whatever the route.
// They index vars[i] against literals[i], and a route reaches them straight
// from a doc comment, since the generator runs on a model the parser rejected.
//
// Agreement with [Vars] is not an invariant. Segments trims a "{$}" suffix
// before it scans and Vars does not, which the two read differently when that
// text is the tail of an unterminated brace: "{{$}" is one variable to Vars
// and none to Segments. Every well-formed route reads the same through both.
func FuzzSegments(f *testing.F) {
	for _, seed := range []string{
		"/", "", "/items/{id}", "/items/{id", "/items/}id", "/items/{{id}",
		"/items/{a}{b}", "/items/{...}", "/items/{id}/{$}/b", "{", "}", "{}{}",
		"{{$}", "/files/{path...}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, route string) {
		literals, vars := Segments(route)
		if len(literals) != len(vars)+1 {
			t.Fatalf("route %q: %d literals, %d vars", route, len(literals), len(vars))
		}
	})
}
