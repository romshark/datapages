package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultVersion tests that the version comes from the redirect of the
// index.html that docpages writes, and that any other redirect fails.
func TestDefaultVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		html    string
		version string
		err     string
	}{
		"main": {
			html:    `<meta http-equiv="refresh" content="0; url=main/">`,
			version: "main",
		},
		"release": {
			html:    `<meta http-equiv="refresh" content="0; url=v1.12/">`,
			version: "v1.12",
		},
		"patch version": {
			html: `<meta http-equiv="refresh" content="0; url=v1.2.3/">`,
			err:  `the index.html of docpages redirects to "v1.2.3/", neither main nor a feature version`,
		},
		"no redirect": {
			html: `<title>Datapages documentation</title>`,
			err:  "the index.html of docpages has no redirect to a version",
		},
	} {
		t.Run(name, func(t *testing.T) {
			version, err := defaultVersion([]byte(tc.html))
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.version, version)
		})
	}
}

// TestInstall tests that the install command takes main for the docs of main
// and latest for those of a release.
func TestInstall(t *testing.T) {
	require.Equal(t, "main", landing{version: "main"}.install())
	require.Equal(t, "latest", landing{version: "v1.2"}.install())
}
