package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
)

// TestStartUpdateCheck tests the notice the CLI prints against a stubbed
// releases endpoint. Only a newer release produces output; the same or an older one,
// and a dev build with no version at all, print nothing.
func TestStartUpdateCheck(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	for name, tc := range map[string]struct {
		responseTag     string
		responseHTMLURL string
		currentVersion  string
		wantOutput      string
	}{
		"newer version available": {
			responseTag:     "v2.0.0",
			responseHTMLURL: "https://example.com/releases/v2.0.0",
			currentVersion:  "v1.0.0",
			wantOutput: "update available: v2.0.0\n  run: go install " +
				datapagesCommandPath +
				"@latest\nchangelog: https://example.com/releases/v2.0.0\n",
		},
		"same version": {
			responseTag:     "v1.0.0",
			responseHTMLURL: "https://example.com/releases/v1.0.0",
			currentVersion:  "v1.0.0",
			wantOutput:      "",
		},
		"older version": {
			responseTag:     "v0.9.0",
			responseHTMLURL: "https://example.com/releases/v0.9.0",
			currentVersion:  "v1.0.0",
			wantOutput:      "",
		},
		"dev build skipped": {
			responseTag:     "v2.0.0",
			responseHTMLURL: "https://example.com/releases/v2.0.0",
			currentVersion:  "",
			wantOutput:      "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(struct {
						TagName string `json:"tag_name"`
						HTMLURL string `json:"html_url"`
					}{TagName: tc.responseTag, HTMLURL: tc.responseHTMLURL})
				}),
			)
			t.Cleanup(srv.Close)

			// Redirect all requests to the test server.
			srvURL, err := url.Parse(srv.URL)
			require.NoError(t, err)
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					req = req.Clone(req.Context())
					req.URL = srvURL
					return http.DefaultTransport.RoundTrip(req)
				}),
			}

			var buf bytes.Buffer
			done := startUpdateCheck(context.Background(),
				tc.currentVersion, &buf, client)
			<-done
			require.Equal(t, tc.wantOutput, buf.String())
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestIsNewerVersion tests the comparison the notice is gated on.
// A prerelease precedes its release, a build suffix is ignored, and an empty version
// on either side is never newer. A dev build must not be told it is out of date.
func TestIsNewerVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		latest  string
		current string
		want    bool
	}{
		"newer major":         {latest: "v2.0.0", current: "v1.9.9", want: true},
		"newer minor":         {latest: "v1.2.0", current: "v1.1.9", want: true},
		"newer patch":         {latest: "v1.1.2", current: "v1.1.1", want: true},
		"same":                {latest: "v1.2.3", current: "v1.2.3", want: false},
		"older":               {latest: "v1.0.0", current: "v1.2.3", want: false},
		"no v prefix":         {latest: "1.2.0", current: "1.1.0", want: true},
		"mixed prefix":        {latest: "v1.2.0", current: "1.1.0", want: true},
		"newer prerelease":    {latest: "v1.2.0-alpha", current: "v1.1.0", want: true},
		"build meta stripped": {latest: "v1.2.0+build", current: "v1.1.0", want: true},
		"empty latest":        {latest: "", current: "v1.0.0", want: false},
		"empty current":       {latest: "v1.0.0", current: "", want: false},
		"release of current prerelease": {
			latest: "v0.11.0", current: "0.11.0-rc.1", want: true,
		},
		"older than current prerelease": {
			latest: "v0.10.1", current: "0.11.0-rc.1", want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := isNewerVersion(tc.latest, tc.current)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestPrintUpdateNotice tests the exact text of the notice, the install command
// it names included, since that is what the user copies.
func TestPrintUpdateNotice(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v1.2.3", "https://example.com/changelog")
	require.Equal(
		t,
		"update available: v1.2.3\n  run: go install "+
			datapagesCommandPath+
			"@latest\nchangelog: https://example.com/changelog\n",
		buf.String(),
	)
}
