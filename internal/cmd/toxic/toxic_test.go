package toxic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
)

var (
	latencyDown = Slot{Type: Latency, Stream: Downstream}
	latencyUp   = Slot{Type: Latency, Stream: Upstream}
	rateDown    = Slot{Type: Bandwidth, Stream: Downstream}
	rateUp      = Slot{Type: Bandwidth, Stream: Upstream}
	limitDown   = Slot{Type: LimitData, Stream: Downstream}
	errorDown   = Slot{Type: HTTPError, Stream: Downstream}
	errorUp     = Slot{Type: HTTPError, Stream: Upstream}
)

func toxicWith(values ...int64) Toxic {
	x := Toxic{Toxicity: 1}
	copy(x.Values[:], values)
	return x
}

// TestSlotText tests that every slot reads back from its name and that an
// unknown name fails.
func TestSlotText(t *testing.T) {
	for _, s := range Slots {
		text, err := s.MarshalText()
		require.NoError(t, err)
		var got Slot
		require.NoError(t, got.UnmarshalText(text))
		require.Equal(t, s, got)
	}
	var s Slot
	require.Error(t, s.UnmarshalText([]byte("latency_sideways")))
}

// TestEffective tests how the overrides of an endpoint combine with the global
// toxics, for the front and for Toxiproxy.
func TestEffective(t *testing.T) {
	disabled := toxicWith(100)
	disabled.Disabled = true
	for name, tc := range map[string]struct {
		global, override Set
		front            bool
		want             []active
	}{
		"global only": {
			global: Set{latencyDown: toxicWith(100), limitDown: toxicWith(8)},
			want: []active{
				{slot: latencyDown, toxic: toxicWith(100)},
				{slot: limitDown, toxic: toxicWith(8)},
			},
		},
		"override replaces its slot": {
			global:   Set{latencyDown: toxicWith(100), limitDown: toxicWith(8)},
			override: Set{latencyDown: toxicWith(900)},
			want: []active{
				{slot: latencyDown, toxic: toxicWith(900)},
				{slot: limitDown, toxic: toxicWith(8)},
			},
		},
		"override turns global off": {
			global:   Set{latencyDown: toxicWith(100)},
			override: Set{latencyDown: disabled},
		},
		"override adds a slot": {
			override: Set{latencyUp: toxicWith(5)},
			want:     []active{{slot: latencyUp, toxic: toxicWith(5)}},
		},
		"disabled global": {
			global: Set{latencyDown: disabled},
		},
		"toxiproxy leaves out the front": {
			global: Set{
				latencyDown: toxicWith(100),
				rateDown:    toxicWith(8),
				errorUp:     toxicWith(500),
			},
			want: []active{{slot: latencyDown, toxic: toxicWith(100)}},
		},
		"front leaves out toxiproxy": {
			global: Set{
				latencyDown: toxicWith(100),
				rateDown:    toxicWith(8),
				errorUp:     toxicWith(500),
			},
			front: true,
			want: []active{
				{slot: rateDown, toxic: toxicWith(8)},
				{slot: errorUp, toxic: toxicWith(500)},
			},
		},
		"front override": {
			global:   Set{errorUp: toxicWith(500), errorDown: toxicWith(502)},
			override: Set{errorUp: toxicWith(503)},
			front:    true,
			want: []active{
				{slot: errorDown, toxic: toxicWith(502)},
				{slot: errorUp, toxic: toxicWith(503)},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, effective(tc.global, tc.override, tc.front))
		})
	}
}

// TestCreateJSON tests the body that creates a toxic in Toxiproxy.
func TestCreateJSON(t *testing.T) {
	x := toxicWith(64, 8, 1500)
	x.Toxicity = 0.25
	a := active{slot: Slot{Type: Slicer, Stream: Upstream}, toxic: x}
	var got map[string]any
	require.NoError(t, json.Unmarshal(a.createJSON(), &got))
	require.Equal(t, map[string]any{
		"name":     "slicer_upstream",
		"type":     "slicer",
		"stream":   "upstream",
		"toxicity": 0.25,
		"attributes": map[string]any{
			"average_size":   float64(64),
			"size_variation": float64(8),
			"delay":          float64(1500),
		},
	}, got)
}

// TestSettingsFromConfig tests the conversion of the presets of datapages.yaml
// into the units of Toxiproxy, the default toxicity of 1, the values a disabled
// toxic with invalid ones takes, and that No throttling is in effect.
func TestSettingsFromConfig(t *testing.T) {
	half := float32(0.5)
	c := &config.WatchToxiproxy{Presets: []config.ToxiproxyPreset{{
		Name: "Slow",
		Toxics: config.Toxics{
			Latency: &config.ToxicStreams[config.ToxicLatency]{
				Downstream: &config.ToxicLatency{
					Latency: 2 * time.Second, Jitter: 5 * time.Millisecond,
				},
			},
			Slicer: &config.ToxicStreams[config.ToxicSlicer]{
				Upstream: &config.ToxicSlicer{
					ToxicBase:   config.ToxicBase{Toxicity: &half},
					AverageSize: 10, SizeVariation: 2, Delay: 3 * time.Millisecond,
				},
			},
			HTTPError: &config.ToxicStreams[config.ToxicHTTPError]{
				Downstream: &config.ToxicHTTPError{
					ToxicBase: config.ToxicBase{Toxicity: &half}, Status: 500,
				},
			},
		},
		Endpoints: map[config.EndpointPattern]config.Toxics{
			"GET /{$}": {
				Latency: &config.ToxicStreams[config.ToxicLatency]{
					Downstream: &config.ToxicLatency{
						ToxicBase: config.ToxicBase{Disabled: true},
					},
				},
				Slicer: &config.ToxicStreams[config.ToxicSlicer]{
					Upstream: &config.ToxicSlicer{
						ToxicBase: config.ToxicBase{Disabled: true},
					},
				},
				Bandwidth: &config.ToxicStreams[config.ToxicBandwidth]{
					Downstream: &config.ToxicBandwidth{
						ToxicBase: config.ToxicBase{Disabled: true},
					},
				},
				HTTPError: &config.ToxicStreams[config.ToxicHTTPError]{
					Upstream: &config.ToxicHTTPError{Status: 503},
					Downstream: &config.ToxicHTTPError{
						ToxicBase: config.ToxicBase{Disabled: true},
					},
				},
			},
		},
	}}}
	slicer := toxicWith(10, 2, 3000)
	slicer.Toxicity = 0.5
	httpError := toxicWith(500)
	httpError.Toxicity = 0.5
	off := func(x Toxic) Toxic { x.Disabled = true; return x }
	none := Settings{
		Global:    Set{},
		Endpoints: map[string]Set{},
		Active:    config.PresetNoThrottling,
	}
	want := none
	want.Presets = []Preset{{
		Name: "Slow",
		Toxics: Set{
			latencyDown:                      toxicWith(2000, 5),
			{Type: Slicer, Stream: Upstream}: slicer,
			errorDown:                        httpError,
		},
		Endpoints: map[string]Set{"GET /{$}": {
			latencyDown:                      off(toxicWith(0, 0)),
			{Type: Slicer, Stream: Upstream}: off(toxicWith(10, 2, 3000)),
			rateDown:                         off(NewToxic(Bandwidth)),
			errorUp:                          toxicWith(503),
			errorDown:                        off(toxicWith(500)),
		}},
	}}
	require.Equal(t, want, SettingsFromConfig(c))
	require.Equal(t, none, SettingsFromConfig(nil))
}

// TestSettingsYAMLRoundTrip tests that [Settings.YAML] loads back into the same
// settings and lists known endpoints first.
func TestSettingsYAMLRoundTrip(t *testing.T) {
	disabled := toxicWith(250, 10)
	disabled.Disabled = true
	partial := toxicWith(1)
	partial.Toxicity = 0.3
	httpError := toxicWith(500)
	httpError.Toxicity = 0.5
	s := Settings{
		Global: Set{}, Endpoints: map[string]Set{}, Active: config.PresetNoThrottling,
		Presets: []Preset{
			{
				Name: "Upload",
				Toxics: Set{
					latencyDown:                        toxicWith(1500, 0),
					rateDown:                           partial,
					{Type: Slicer, Stream: Downstream}: toxicWith(64, 8, 1500),
					{Type: Timeout, Stream: Upstream}:  toxicWith(0),
					errorUp:                            httpError,
				},
				Endpoints: map[string]Set{
					"POST /upload/{$}": {
						latencyDown: disabled,
						limitDown:   toxicWith(1024),
						errorDown:   toxicWith(503),
					},
					"GET /gone/{$}":      {latencyUp: toxicWith(7, 0)},
					"GET /post/{id}/{$}": {},
				},
			},
			{
				Name:   `Train "tunnel": 2`,
				Toxics: Set{latencyDown: toxicWith(900, 300)},
				Endpoints: map[string]Set{
					"POST /upload/{$}": {limitDown: toxicWith(64)},
				},
			},
			{Name: "Clean", Toxics: Set{}, Endpoints: map[string]Set{}},
		},
	}
	got := s.YAML(4, []string{"POST /upload/{$}"})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datapages.yaml"),
		[]byte("watch:\n  toxiproxy:\n    host: localhost:9090\n"+got), 0o644))
	c, _, err := config.Load(dir)
	require.NoError(t, err, "YAML:\n%s", got)

	want := s.Clone()
	// Without overrides it writes nothing.
	delete(want.Presets[0].Endpoints, "GET /post/{id}/{$}")
	require.Equal(t, want, SettingsFromConfig(c.Watch.Toxiproxy))
	known := strings.Index(got, `"POST /upload/{$}"`)
	unknown := strings.Index(got, `"GET /gone/{$}"`)
	require.True(t, known >= 0 && unknown > known,
		"a known endpoint comes first:\n%s", got)
}

// TestToxicValidate tests the values the settings page rejects before Toxiproxy would.
func TestToxicValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		typ     Type
		toxic   Toxic
		wantErr string
	}{
		"latency": {typ: Latency, toxic: toxicWith(100, 10)},
		"negative latency": {
			typ: Latency, toxic: toxicWith(-1, 0),
			wantErr: "latency must be at least 0",
		},
		"latency of 24h": {typ: Latency, toxic: toxicWith(86_400_000, 86_400_000)},
		"latency above 24h": {
			typ: Latency, toxic: toxicWith(86_400_001, 0),
			wantErr: "latency must be at most 86400000",
		},
		"latency overflowing time.Duration": {
			typ: Latency, toxic: toxicWith(10_000_000_000_000, 0),
			wantErr: "latency must be at most 86400000",
		},
		"jitter above 24h": {
			typ: Latency, toxic: toxicWith(0, 86_400_001),
			wantErr: "jitter must be at most 86400000",
		},
		"timeout above 24h": {
			typ: Timeout, toxic: toxicWith(86_400_001),
			wantErr: "timeout must be at most 86400000",
		},
		"zero rate": {
			typ: Bandwidth, toxic: toxicWith(0),
			wantErr: "rate must be at least 1",
		},
		"rate of 10 GB/s": {typ: Bandwidth, toxic: toxicWith(10_000_000)},
		"rate above 10 GB/s": {
			typ: Bandwidth, toxic: toxicWith(10_000_001),
			wantErr: "rate must be at most 10000000",
		},
		"slicer": {typ: Slicer, toxic: toxicWith(10, 10, 0)},
		"slicer delay above 24h": {
			typ: Slicer, toxic: toxicWith(10, 0, 86_400_000_001),
			wantErr: "delay must be at most 86400000000",
		},
		"slicer variation": {
			typ: Slicer, toxic: toxicWith(10, 11, 0),
			wantErr: "variation",
		},
		"slicer zero size": {
			typ: Slicer, toxic: toxicWith(0, 0, 0),
			wantErr: "average size",
		},
		"status": {typ: HTTPError, toxic: toxicWith(599)},
		"status below 400": {
			typ: HTTPError, toxic: toxicWith(399),
			wantErr: "status must be at least 400",
		},
		"status above 599": {
			typ: HTTPError, toxic: toxicWith(600),
			wantErr: "status must be at most 599",
		},
		"toxicity above one": {
			typ: LimitData, toxic: Toxic{Toxicity: 2},
			wantErr: "toxicity",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.toxic.Validate(tc.typ)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
