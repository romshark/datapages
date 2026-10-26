package toxic

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
)

// TestDefaultPresets tests the presets against the formulas of Chrome DevTools,
// in bytes per second and milliseconds, rounded to the units of Toxiproxy.
func TestDefaultPresets(t *testing.T) {
	chrome := []struct {
		name              string
		latency, down, up float64
	}{
		{name: "Fast 4G", latency: 60 * 2.75, down: 9e6 / 8 * .9, up: 1.5e6 / 8 * .9},
		{name: "Slow 4G", latency: 150 * 3.75, down: 1.6e6 / 8 * .9, up: 750e3 / 8 * .9},
		{name: "3G", latency: 400 * 5, down: 500e3 / 8 * .8, up: 500e3 / 8 * .8},
	}
	presets := DefaultPresets()
	require.Len(t, presets, len(chrome))
	for i, c := range chrome {
		p := presets[i]
		require.Equal(t, c.name, p.Name)
		require.Equal(t, Set{
			latencyDown:                         toxicWith(int64(math.Round(c.latency))),
			rateDown:                            toxicWith(int64(math.Round(c.down / 1000))),
			{Type: Bandwidth, Stream: Upstream}: toxicWith(int64(math.Round(c.up / 1000))),
		}, p.Toxics, c.name)
		for slot, x := range p.Toxics {
			require.NoError(t, x.Validate(slot.Type), c.name)
		}
	}
}

// TestAllPresets tests that No throttling, without toxics, comes before the
// presets of the settings and Offline, which resets every connection, after
// them, and that only those 2 count as built in.
func TestAllPresets(t *testing.T) {
	all := Settings{Presets: []Preset{{Name: "Train", Toxics: Set{}}}}.AllPresets()
	none := map[string]Set{}
	require.Equal(t, []Preset{
		{Name: "No throttling", Toxics: Set{}, Endpoints: none},
		{Name: "Train", Toxics: Set{}},
		{Name: "Offline", Endpoints: none, Toxics: Set{
			{Type: ResetPeer, Stream: Upstream}:   toxicWith(0),
			{Type: ResetPeer, Stream: Downstream}: toxicWith(0),
		}},
	}, all)
	require.True(t, IsBuiltinPreset("No throttling"))
	require.True(t, IsBuiltinPreset("Offline"))
	require.False(t, IsBuiltinPreset("Train"))
}

// TestActivePreset tests which preset the global toxics with the overrides
// match, the built-in ones included: a toxic turned off counts as none, an
// override that changes nothing as no override, any other difference matches
// no preset, and of several presets that match, the one applied last.
func TestActivePreset(t *testing.T) {
	presets := append(DefaultPresets(), Preset{Name: "Copy", Toxics: DefaultPresets()[1].Toxics})
	slow := presets[1].Toxics
	off := toxicWith(100)
	off.Disabled = true
	partial := slow[latencyDown]
	partial.Toxicity = 0.5
	withSlow := func(slot Slot, x Toxic) Set {
		s := Set{slot: x}
		for k, v := range slow {
			if k != slot {
				s[k] = v
			}
		}
		return s
	}
	const ep = "GET /{$}"
	for name, tc := range map[string]struct {
		global    Set
		endpoints map[string]Set
		active    string
		want      string
	}{
		"none on":      {global: Set{}, want: "No throttling"},
		"only off":     {global: Set{latencyDown: off}, want: "No throttling"},
		"slow 4G":      {global: withSlow(limitDown, off), want: "Slow 4G"},
		"applied copy": {global: slow, active: "Copy", want: "Copy"},
		"changed copy": {global: Set{}, active: "Copy", want: "No throttling"},
		"offline":      {global: offline().Toxics, want: "Offline"},
		"offline, override": {
			global:    offline().Toxics,
			endpoints: map[string]Set{ep: {latencyUp: toxicWith(5, 0)}},
		},
		"override as global": {
			global:    slow,
			endpoints: map[string]Set{ep: {latencyDown: slow[latencyDown]}},
			want:      "Slow 4G",
		},
		"override turns off": {
			global:    slow,
			endpoints: map[string]Set{ep: {latencyDown: off}},
		},
		"other toxicity": {global: withSlow(latencyDown, partial)},
		"extra toxic":    {global: withSlow(limitDown, toxicWith(10))},
	} {
		t.Run(name, func(t *testing.T) {
			s := Settings{
				Global: tc.global, Endpoints: tc.endpoints,
				Presets: presets, Active: tc.active,
			}
			require.Equal(t, tc.want, s.ActivePreset())
		})
	}
}

// TestPresetCommands tests applying a preset, built in or not, which replaces
// the global toxics and the overrides, creating one, which copies them and
// applies it, and deleting one, which keeps the toxics. A built-in preset
// keeps its name and stays.
func TestPresetCommands(t *testing.T) {
	s := Settings{
		Global:    Set{limitDown: toxicWith(10)},
		Endpoints: map[string]Set{"GET /{$}": {latencyUp: toxicWith(5, 0)}},
		Presets:   DefaultPresets(),
	}
	require.NoError(t, s.ApplyPreset("3G"))
	require.Equal(t, DefaultPresets()[2].Toxics, s.Global)
	require.Equal(t, "3G", s.Active)
	require.Equal(t, map[string]Set{}, s.Endpoints, "3G has no overrides")
	require.ErrorIs(t, s.ApplyPreset("5G"), ErrNoPreset)
	require.NoError(t, s.ApplyPreset("Offline"))
	require.Equal(t, offline().Toxics, s.Global)
	require.NoError(t, s.ApplyPreset("No throttling"))
	require.Empty(t, s.Global)

	tiny := Set{limitDown: toxicWith(10)}
	upload := map[string]Set{"POST /upload/{$}": {latencyUp: toxicWith(5, 0)}}
	require.NoError(t, s.CreatePreset("  Tiny  ", tiny, upload))
	require.Equal(t, Preset{
		Name:      "Tiny",
		Toxics:    Set{limitDown: toxicWith(10)},
		Endpoints: map[string]Set{"POST /upload/{$}": {latencyUp: toxicWith(5, 0)}},
	}, s.Presets[3])
	require.Equal(t, tiny, s.Global)
	require.Equal(t, upload, s.Endpoints)
	require.Equal(t, "Tiny", s.Active)
	tiny[limitDown] = toxicWith(20)
	upload["POST /upload/{$}"][latencyUp] = toxicWith(6, 0)
	s.Global[limitDown] = toxicWith(30)
	s.Endpoints["POST /upload/{$}"][latencyUp] = toxicWith(7, 0)
	require.Equal(t, toxicWith(10), s.Presets[3].Toxics[limitDown],
		"the preset holds a copy")
	require.Equal(t, toxicWith(5, 0), s.Presets[3].Endpoints["POST /upload/{$}"][latencyUp],
		"the preset holds a copy of the overrides")

	require.NoError(t, s.CreatePreset("Train", DefaultPresets()[1].Toxics, nil))
	require.Equal(t, "Train", s.ActivePreset(), "the copy, not Slow 4G")
	require.NoError(t, s.ApplyPreset("Tiny"))
	require.Equal(t,
		map[string]Set{"POST /upload/{$}": {latencyUp: toxicWith(5, 0)}}, s.Endpoints)
	require.NoError(t, s.ApplyPreset("Train"))

	require.ErrorIs(t, s.CreatePreset("Fast 4G", Set{}, nil), ErrPresetExists)
	require.ErrorIs(t, s.CreatePreset("Offline", Set{}, nil), ErrPresetExists)
	require.ErrorIs(t, s.CreatePreset(" ", Set{}, nil), ErrPresetUnnamed)
	require.Len(t, s.Presets, 5)

	require.NoError(t, s.DeletePreset("Train"))
	require.Empty(t, s.Active)
	require.Equal(t, DefaultPresets()[1].Toxics, s.Global)
	require.NoError(t, s.DeletePreset("Fast 4G"))
	require.Equal(t, "Slow 4G", s.Presets[0].Name)
	require.ErrorIs(t, s.DeletePreset("Fast 4G"), ErrNoPreset)
	require.ErrorIs(t, s.DeletePreset("No throttling"), ErrPresetBuiltin)
	require.ErrorIs(t, s.DeletePreset("Offline"), ErrPresetBuiltin)

	p, err := DefaultPreset("3G")
	require.NoError(t, err)
	require.Equal(t, DefaultPresets()[2], p)
	_, err = DefaultPreset("Offline")
	require.ErrorIs(t, err, ErrNoPreset)
}

// TestScaffoldPresets tests the datapages.yaml init writes: it has the presets
// of Datapages and no toxics, which selects the built-in preset No throttling,
// and it holds them as the settings page saves them, which a save leaves as
// they are.
func TestScaffoldPresets(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, config.WriteDefault(dir))
	c, _, err := config.Load(dir)
	require.NoError(t, err)
	s := SettingsFromConfig(c.Watch.Toxiproxy)
	require.Equal(t, DefaultPresets(), s.Presets)
	require.Empty(t, s.Global)
	require.Equal(t, "No throttling", s.ActivePreset())
	require.True(t, strings.HasSuffix(config.DefaultYAML, "  toxiproxy:\n"+s.YAML(4, nil)),
		"the settings page writes:\n%s", s.YAML(4, nil))
}

// TestPresetsYAML tests that settings without presets write none.
func TestPresetsYAML(t *testing.T) {
	s := Settings{Global: Set{}, Endpoints: map[string]Set{}}
	require.Empty(t, s.YAML(0, nil))
}
