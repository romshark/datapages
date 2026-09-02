package urlpath_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser/internal/urlpath"
)

// TestClean tests the form a declared path is compared in, which is why the trailing
// slashes go: "/foo" and "/foo/" are one route and have to collide as duplicates.
// "/" is the exception and keeps its slash.
func TestClean(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  string
	}{
		"root":              {input: "/", want: "/"},
		"empty":             {input: "", want: ""},
		"no trailing":       {input: "/foo", want: "/foo"},
		"single trailing":   {input: "/foo/", want: "/foo"},
		"double trailing":   {input: "/foo//", want: "/foo"},
		"nested":            {input: "/foo/bar", want: "/foo/bar"},
		"nested trailing":   {input: "/foo/bar/", want: "/foo/bar"},
		"only slashes":      {input: "//", want: ""},
		"relative":          {input: "foo/", want: "foo"},
		"relative no slash": {input: "foo", want: "foo"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, urlpath.Clean(tc.input))
		})
	}
}
