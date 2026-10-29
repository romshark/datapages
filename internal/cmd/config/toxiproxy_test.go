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
          data-cap:
            downstream:
              volume: 2000000000
              rate: 16
        endpoints:
          "POST /upload/{$}":
            client-bandwidth:
              upstream:
                rate: 8
                per: request
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
	// indent returns the lines, each prefixed with pad and ended with a newline.
	indent := func(pad string, lines []string) string {
		var b strings.Builder
		for _, l := range lines {
			b.WriteString(pad)
			b.WriteString(l)
			b.WriteByte('\n')
		}
		return b.String()
	}
	// toxiproxy returns datapages.yaml with the lines in watch.toxiproxy.
	toxiproxy := func(lines ...string) string {
		return "watch:\n  toxiproxy:\n" + indent("    ", lines)
	}
	// preset returns datapages.yaml with the lines in the preset P.
	preset := func(lines ...string) string {
		return toxiproxy("presets:", "  - name: P") + indent("        ", lines)
	}
	// toxic returns datapages.yaml with the preset P and 1 toxic of typ on
	// stream with the fields.
	toxic := func(typ, stream string, fields ...string) string {
		return preset("toxics:", "  "+typ+":", "    "+stream+":") +
			indent("              ", fields)
	}
	for name, tc := range map[string]struct {
		yaml    string
		wantErr string
	}{
		"valid": {yaml: valid},
		"toxics outside a preset": {
			yaml: toxiproxy(
				"toxics:",
				"  latency:",
				"    downstream:",
				"      latency: 1s",
			),
			wantErr: "toxics",
		},
		"host without port": {
			yaml:    toxiproxy("host: localhost"),
			wantErr: `invalid host "localhost"`,
		},
		"ui-host without port": {
			yaml:    toxiproxy("ui-host: localhost"),
			wantErr: `invalid ui-host "localhost"`,
		},
		"ui-host on host": {
			yaml:    toxiproxy("host: :9090", "ui-host: :9090"),
			wantErr: `host and ui-host are both ":9090"`,
		},
		"toxicity above 1": {
			yaml:    toxic("latency", "downstream", "toxicity: 1.5"),
			wantErr: "toxicity 1.5 must be from 0 to 1",
		},
		"latency below a millisecond": {
			yaml:    toxic("latency", "downstream", "latency: 1500us"),
			wantErr: "whole milliseconds",
		},
		"negative timeout": {
			yaml:    toxic("timeout", "upstream", "timeout: -1s"),
			wantErr: "whole milliseconds",
		},
		"latency above 24h": {
			yaml:    toxic("latency", "downstream", "latency: 24h1ms"),
			wantErr: "latency 24h0m0.001s must be whole milliseconds from 0 to 24h0m0s",
		},
		"jitter above 24h": {
			yaml:    toxic("latency", "downstream", "jitter: 25h"),
			wantErr: "jitter 25h0m0s must be whole milliseconds from 0 to 24h0m0s",
		},
		"zero rate": {
			yaml:    toxic("client-bandwidth", "downstream", "rate: 0"),
			wantErr: "rate 0 must be from 1 to 10000000000 B/s",
		},
		"rate above 10 GB/s": {
			yaml:    toxic("client-bandwidth", "downstream", "rate: 10000000001"),
			wantErr: "rate 10000000001 must be from 1 to 10000000000 B/s",
		},
		"zero server rate": {
			yaml:    toxic("server-bandwidth", "upstream", "rate: 0"),
			wantErr: "rate 0 must be from 1 to 10000000000 B/s",
		},
		"server per": {
			yaml:    toxic("server-bandwidth", "upstream", "rate: 8", "per: client"),
			wantErr: "per",
		},
		"unknown per": {
			yaml:    toxic("client-bandwidth", "downstream", "rate: 8", "per: tab"),
			wantErr: `invalid per "tab", use: client, request`,
		},
		"negative volume": {
			yaml:    toxic("data-cap", "downstream", "volume: -1", "rate: 8"),
			wantErr: "volume -1 must be 0 or more",
		},
		"data cap without rate": {
			yaml:    toxic("data-cap", "upstream", "volume: 1024"),
			wantErr: "rate 0 must be from 1 to 10000000000 B/s",
		},
		"slicer variation above average": {
			yaml: toxic(
				"slicer", "downstream", "average-size: 8", "size-variation: 9",
			),
			wantErr: "size-variation 9 must be from 0 to average-size",
		},
		"slicer delay below a microsecond": {
			yaml:    toxic("slicer", "downstream", "average-size: 8", "delay: 1500ns"),
			wantErr: "whole microseconds",
		},
		"slicer delay above 24h": {
			yaml:    toxic("slicer", "downstream", "average-size: 8", "delay: 25h"),
			wantErr: "delay 25h0m0s must be whole microseconds from 0 to 24h0m0s",
		},
		"negative limit": {
			yaml:    toxic("limit-data", "downstream", "bytes: -1"),
			wantErr: "bytes -1 must be 0 or more",
		},
		"status below 400": {
			yaml:    toxic("http-error", "upstream", "status: 302"),
			wantErr: "status 302 must be from 400 to 599",
		},
		"status above 599": {
			yaml:    toxic("http-error", "downstream", "status: 600"),
			wantErr: "status 600 must be from 400 to 599",
		},
		"preset without name": {
			yaml:    toxiproxy("presets:", `  - name: " "`),
			wantErr: "a preset has no name",
		},
		"preset built in": {
			yaml:    toxiproxy("presets:", "  - name: Unreachable"),
			wantErr: `the preset "Unreachable" is built in`,
		},
		"presets of 1 name": {
			yaml:    toxiproxy("presets:", "  - name: Slow", "  - name: Slow"),
			wantErr: `2 presets are named "Slow"`,
		},
		"endpoint without method": {
			yaml: preset(
				"endpoints:",
				`  "/upload/":`,
				"    latency:",
				"      downstream:",
				"        latency: 1s",
			),
			wantErr: "must start with a method",
		},
		"endpoint pattern invalid": {
			yaml: preset(
				"endpoints:",
				`  "GET /{a":`,
				"    latency:",
				"      downstream:",
				"        latency: 1s",
			),
			wantErr: "invalid route pattern",
		},
		"unknown toxic": {
			yaml:    toxic("packet-loss", "downstream", "rate: 1"),
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
			require.Equal(t,
				ToxicDataCap{Volume: 2_000_000_000, Rate: 16},
				*p.Toxics.DataCap.Downstream)
			ep := p.Endpoints["POST /upload/{$}"]
			require.Equal(t, int64(8), ep.ClientBandwidth.Upstream.Rate)
			require.Equal(t, ToxicPerRequest, ep.ClientBandwidth.Upstream.Per)
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

// TestLoadToxiproxyDefaults tests that every setting of watch.toxiproxy is optional.
func TestLoadToxiproxyDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datapages.yaml"),
		[]byte("watch:\n  toxiproxy:\n    disabled: true\n"), 0o644))
	c, _, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, &WatchToxiproxy{Disabled: true}, c.Watch.Toxiproxy)
}
