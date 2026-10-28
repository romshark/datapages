package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const section = "presets:\n  - name: Slow\n    toxics:\n" +
	"      latency:\n        downstream:\n          latency: 5ms\n"

// TestSetToxiproxy tests that setToxiproxy replaces or adds the
// watch.toxiproxy section and keeps the rest of the file.
func TestSetToxiproxy(t *testing.T) {
	for name, tc := range map[string]struct {
		src, section, want string
	}{
		"replaces the section of the scaffold": {
			src:     DefaultYAML + "  # after\n  debounce: 1s\n",
			section: section,
			want: `cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
  # The app through Toxiproxy on the port after the dev server, as
  # http://localhost:7332, and its settings page on the port after that.
  # The presets are the throttling presets of Chrome DevTools.
  toxiproxy:
    presets:
      - name: Slow
        toxics:
          latency:
            downstream:
              latency: 5ms
  # after
  debounce: 1s
`,
		},
		"keeps the blank lines after the section": {
			src: "watch:\n  toxiproxy:\n    host: localhost:9090\n\n\n" +
				"cmd: cmd/server\n",
			section: "host: localhost:9091\n",
			want: "watch:\n  toxiproxy:\n    host: localhost:9091\n\n\n" +
				"cmd: cmd/server\n",
		},
		"replaces a section at the end without a line break": {
			src:     "watch:\n  toxiproxy:\n    disabled: false",
			section: section,
			want: "watch:\n  toxiproxy:\n    presets:\n      - name: Slow\n" +
				"        toxics:\n          latency:\n" +
				"            downstream:\n              latency: 5ms\n",
		},
		"adds the section after the last key of watch": {
			src:     "watch:\n  debounce: 1s\ncmd: cmd/server\n",
			section: "host: localhost:9090\n",
			want: "watch:\n  debounce: 1s\n  toxiproxy:\n    host: localhost:9090\n" +
				"cmd: cmd/server\n",
		},
		"adds watch to a file without it": {
			src:     "cmd: cmd/server",
			section: "host: localhost:9090\n",
			want:    "cmd: cmd/server\nwatch:\n  toxiproxy:\n    host: localhost:9090\n",
		},
		"fills a watch without a value": {
			src:     "watch:\ncmd: cmd/server\n",
			section: "host: localhost:9090\n",
			want:    "watch:\n  toxiproxy:\n    host: localhost:9090\ncmd: cmd/server\n",
		},
		"writes an empty file": {
			section: "host: localhost:9090\n",
			want:    "watch:\n  toxiproxy:\n    host: localhost:9090\n",
		},
		"keeps an empty section": {
			src:  "watch:\n  toxiproxy:\n    host: localhost:9090\n",
			want: "watch:\n  toxiproxy: {}\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := setToxiproxy([]byte(tc.src), tc.section)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}

// TestSetToxiproxyFile tests that SetToxiproxy writes the existing
// configuration file, datapages.yaml when there is none, and nothing when the
// result would not load.
func TestSetToxiproxyFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SetToxiproxy(dir, section))
	c, found, err := Load(dir)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, c.Watch.Toxiproxy.Presets[0].Toxics.Latency)

	dir = t.TempDir()
	yml := filepath.Join(dir, "datapages.yml")
	require.NoError(t, os.WriteFile(yml, []byte("cmd: cmd/server\n"), 0o644))
	require.NoError(t, SetToxiproxy(dir, section))
	require.NoFileExists(t, filepath.Join(dir, "datapages.yaml"))
	b, err := os.ReadFile(yml)
	require.NoError(t, err)
	require.Contains(t, string(b), "latency: 5ms")

	err = SetToxiproxy(dir, "nonsense: 1\n")
	require.ErrorContains(t, err, "would not load")
	after, err := os.ReadFile(yml)
	require.NoError(t, err)
	require.Equal(t, b, after)
}
