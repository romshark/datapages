package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLoadToxiproxy tests that watch.toxiproxy loads and that Load rejects
// every value Toxiproxy would reject or truncate.
func TestLoadToxiproxy(t *testing.T) {
	const valid = `watch:
  toxiproxy:
    host: localhost:9090
    ui-host: localhost:9091
    presets:
      - name: Upload
        toxics:
          latency:
            downstream:
              latency: 300ms
              jitter: 50ms
              toxicity: 0.5
          slicer:
            upstream:
              average-size: 64
              size-variation: 32
              delay: 1500us
        endpoints:
          "POST /upload/{$}":
            bandwidth:
              upstream:
                rate: 8
            latency:
              downstream:
                disabled: true
            slicer:
              downstream:
                disabled: true
            http-error:
              upstream:
                status: 503
                toxicity: 0.5
              downstream:
                disabled: true
      - name: Clean
`
	// inPreset returns the YAML of a preset whose key holds body, which is
	// indented as directly under watch.toxiproxy.
	inPreset := func(key, body string) string {
		return "watch:\n  toxiproxy:\n    host: :9090\n    presets:\n" +
			"      - name: P\n        " + key + ":\n" +
			"    " + strings.ReplaceAll(strings.TrimSuffix(body, "\n"), "\n", "\n    ") + "\n"
	}
	for name, tc := range map[string]struct {
		yaml    string
		wantErr string
	}{
		"valid": {yaml: valid},
		"toxics outside a preset": {
			yaml: "watch:\n  toxiproxy:\n    toxics:\n" +
				"      latency:\n        downstream:\n          latency: 1s\n",
			wantErr: "toxics",
		},
		"host without port": {
			yaml:    "watch:\n  toxiproxy:\n    host: localhost\n",
			wantErr: `invalid host "localhost"`,
		},
		"ui-host without port": {
			yaml:    "watch:\n  toxiproxy:\n    ui-host: localhost\n",
			wantErr: `invalid ui-host "localhost"`,
		},
		"ui-host on host": {
			yaml:    "watch:\n  toxiproxy:\n    host: :9090\n    ui-host: :9090\n",
			wantErr: `host and ui-host are both ":9090"`,
		},
		"toxicity above 1": {
			yaml:    inPreset("toxics", "      latency:\n        downstream:\n          toxicity: 1.5\n"),
			wantErr: "toxicity 1.5 must be from 0 to 1",
		},
		"latency below a millisecond": {
			yaml:    inPreset("toxics", "      latency:\n        downstream:\n          latency: 1500us\n"),
			wantErr: "whole milliseconds",
		},
		"negative timeout": {
			yaml:    inPreset("toxics", "      timeout:\n        upstream:\n          timeout: -1s\n"),
			wantErr: "whole milliseconds",
		},
		"zero rate": {
			yaml:    inPreset("toxics", "      bandwidth:\n        downstream:\n          rate: 0\n"),
			wantErr: "rate 0 must be at least 1 KB/s",
		},
		"slicer variation above average": {
			yaml: inPreset("toxics", "      slicer:\n        downstream:\n"+
				"          average-size: 8\n          size-variation: 9\n"),
			wantErr: "size-variation 9 must be from 0 to average-size",
		},
		"slicer delay below a microsecond": {
			yaml: inPreset("toxics", "      slicer:\n        downstream:\n"+
				"          average-size: 8\n          delay: 1500ns\n"),
			wantErr: "whole microseconds",
		},
		"negative limit": {
			yaml:    inPreset("toxics", "      limit-data:\n        downstream:\n          bytes: -1\n"),
			wantErr: "bytes -1 must be 0 or more",
		},
		"status below 400": {
			yaml:    inPreset("toxics", "      http-error:\n        upstream:\n          status: 302\n"),
			wantErr: "status 302 must be from 400 to 599",
		},
		"status above 599": {
			yaml:    inPreset("toxics", "      http-error:\n        downstream:\n          status: 600\n"),
			wantErr: "status 600 must be from 400 to 599",
		},
		"preset without name": {
			yaml: "watch:\n  toxiproxy:\n    host: :9090\n    presets:\n" +
				"      - name: \" \"\n",
			wantErr: "a preset has no name",
		},
		"preset built in": {
			yaml: "watch:\n  toxiproxy:\n    host: :9090\n    presets:\n" +
				"      - name: Unreachable\n",
			wantErr: `the preset "Unreachable" is built in`,
		},
		"presets of 1 name": {
			yaml: "watch:\n  toxiproxy:\n    host: :9090\n    presets:\n" +
				"      - name: Slow\n      - name: Slow\n",
			wantErr: `2 presets are named "Slow"`,
		},
		"endpoint without method": {
			yaml: inPreset("endpoints", "      \"/upload/\":\n        latency:\n"+
				"          downstream:\n            latency: 1s\n"),
			wantErr: "must start with a method",
		},
		"endpoint pattern invalid": {
			yaml: inPreset("endpoints", "      \"GET /{a\":\n        latency:\n"+
				"          downstream:\n            latency: 1s\n"),
			wantErr: "invalid route pattern",
		},
		"unknown toxic": {
			yaml:    inPreset("toxics", "      packet-loss:\n        downstream:\n          rate: 1\n"),
			wantErr: "packet-loss",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, "datapages.yaml"), []byte(tc.yaml), 0o644,
			))
			c, _, err := Load(dir)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tp := c.Watch.Toxiproxy
			require.Equal(t, "localhost:9090", tp.Host)
			require.Equal(t, "localhost:9091", tp.UIHost)
			require.Len(t, tp.Presets, 2)
			p := tp.Presets[0]
			require.Equal(t, "Upload", p.Name)
			require.Equal(t, 300*time.Millisecond, p.Toxics.Latency.Downstream.Latency)
			require.Equal(t, float32(0.5), *p.Toxics.Latency.Downstream.Toxicity)
			require.Nil(t, p.Toxics.Latency.Upstream)
			require.Equal(t, 1500*time.Microsecond, p.Toxics.Slicer.Upstream.Delay)
			ep := p.Endpoints["POST /upload/{$}"]
			require.Equal(t, int64(8), ep.Bandwidth.Upstream.Rate)
			require.True(t, ep.Latency.Downstream.Disabled)
			// A disabled toxic needs no other values.
			require.True(t, ep.Slicer.Downstream.Disabled)
			require.Zero(t, ep.Slicer.Downstream.AverageSize)
			require.Equal(t, int64(503), ep.HTTPError.Upstream.Status)
			require.True(t, ep.HTTPError.Downstream.Disabled)
			require.Zero(t, ep.HTTPError.Downstream.Status)
			require.Equal(t, ToxiproxyPreset{Name: "Clean"}, tp.Presets[1])
		})
	}
}

// TestLoadToxiproxyDefaults tests that every setting of watch.toxiproxy is
// optional.
func TestLoadToxiproxyDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datapages.yaml"),
		[]byte("watch:\n  toxiproxy:\n    disabled: true\n"), 0o644))
	c, _, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, &WatchToxiproxy{Disabled: true}, c.Watch.Toxiproxy)
}
