package app_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// TestSignInRedirect tests that signing in navigates to next only when it is
// a path on this site, and to the shows page otherwise.
func TestSignInRedirect(t *testing.T) {
	repo := domain.NewRepository()
	newUser(t, repo, "visitor")
	srv := newServer(t, repo)

	tests := map[string]struct{ next, want string }{
		"path":              {"/tickets/", "/tickets/"},
		"empty":             {"", "/"},
		"absolute":          {"https://evil.example/", "/"},
		"protocol-relative": {"//evil.example/", "/"},
		"three slashes":     {"///evil.example/", "/"},
		"backslash":         {`/\evil.example/`, "/"},
		"tab":               {"/\t/evil.example/", "/"},
		"newline":           {"/\n/evil.example/", "/"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp, body := visitor{srv: srv}.post(t, "/login/submit/",
				map[string]string{
					"emailorusername": "visitor",
					"password":        password,
					"next":            tc.next,
				})
			require.Equal(t, http.StatusOK, resp.StatusCode, body)
			require.Equal(t, tc.want, navigationTarget(t, body))
		})
	}
}

var windowLocation = regexp.MustCompile(`window\.location\s*=\s*("(?:[^"\\]|\\.)*")`)

// navigationTarget returns the URL that the JavaScript of a redirect action
// assigns to window.location.
func navigationTarget(t *testing.T, script string) string {
	t.Helper()
	m := windowLocation.FindStringSubmatch(script)
	require.NotNil(t, m, "no navigation in response:\n%s", script)
	var target string
	require.NoError(t, json.Unmarshal([]byte(m[1]), &target),
		"decoding navigation target %s", m[1])
	return target
}
