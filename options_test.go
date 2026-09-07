package datapages_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestWithDatastarJS tests the URL that goes into the src attribute of a script
// tag in the head of every page. A quote in it closes the attribute and turns
// the rest of the value into further attributes on that tag.
func TestWithDatastarJS(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		src string
		ok  bool
	}{
		"https":             {"https://cdn.example.com/ds.js", true},
		"http":              {"http://cdn.example.com/ds.js", true},
		"root relative":     {"/static/ds.js", true},
		"relative":          {"static/ds.js", true},
		"protocol relative": {"//cdn.example.com/ds.js", true},
		"empty":             {"", false},
		"other scheme":      {"javascript:alert(1)", false},
		"double quote":      {`https://x/ds.js" onload="alert(1)`, false},
		"single quote":      {"https://x/ds.js' onload='alert(1)", false},
		"angle bracket":     {"https://x/ds.js></script><script>", false},
		"space":             {"https://x/d s.js", false},
		"control character": {"https://x/ds.js\nfoo", false},
	} {
		t.Run(name, func(t *testing.T) {
			var cfg datapages.ServerConfig
			err := datapages.WithDatastarJS(tc.src)(&cfg)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, tc.src, cfg.DatastarJS)
				return
			}
			require.ErrorContains(t, err, "WithDatastarJS")
		})
	}
}
