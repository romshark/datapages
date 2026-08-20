package routepattern

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

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
		"empty braces":   {"/{}", []string{"/", "/"}, nil},
		"unclosed brace": {"/items/{id", []string{"/items/"}, nil},
		"empty string":   {"", []string{"/"}, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			literals, vars := Segments(tt.route)
			require.Equal(t, tt.wantLiterals, literals)
			require.Equal(t, tt.wantVars, vars)
		})
	}
}
