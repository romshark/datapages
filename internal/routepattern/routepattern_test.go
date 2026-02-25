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
