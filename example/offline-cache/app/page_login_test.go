package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
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
			login := srv.Visitor(t).Load(t, href.PageLogin(href.QueryPageLogin{}))
			resp := login.Do(t, http.MethodPost, "/login/submit/", map[string]string{
				"emailorusername": "visitor",
				"password":        password,
				"next":            tc.next,
			})
			require.Equal(t, http.StatusOK, resp.Status, resp.Body)
			require.Equal(t, tc.want, resp.Redirect(t))
		})
	}
}
