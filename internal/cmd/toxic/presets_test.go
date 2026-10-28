package toxic

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/config"
)

// TestDefaultPresets tests the presets against the formulas of Chrome DevTools,
// rounded to the units of Toxiproxy.
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
			latencyDown: toxicWith(
				int64(math.Round(c.latency)),
			),
			rateDown: toxicWith(
				int64(math.Round(c.down / 1000)),
			),
			{Type: Bandwidth, Stream: Upstream}: toxicWith(
				int64(math.Round(c.up / 1000)),
			),
		}, p.Toxics, c.name)
		for slot, x := range p.Toxics {
			require.NoError(t, x.Validate(slot.Type), c.name)
		}
	}
}

// TestAllPresets tests that No throttling comes before the presets of the
// settings, Unreachable after them, and that only those 2 are built in.
func TestAllPresets(t *testing.T) {
	all := Settings{Presets: []Preset{{Name: "Train", Toxics: Set{}}}}.AllPresets()
	none := map[string]Set{}
	require.Equal(t, []Preset{
		{Name: "No throttling", Toxics: Set{}, Endpoints: none},
		{Name: "Train", Toxics: Set{}},
		{Name: "Unreachable", Endpoints: none, Toxics: Set{
			{Type: ResetPeer, Stream: Upstream}:   toxicWith(0),
			{Type: ResetPeer, Stream: Downstream}: toxicWith(0),
		}},
	}, all)
	require.True(t, IsBuiltinPreset("No throttling"))
	require.True(t, IsBuiltinPreset("Unreachable"))
	require.False(t, IsBuiltinPreset("Offline"))
	require.False(t, IsBuiltinPreset("Train"))
}

// TestRecord tests that a change of the toxics goes into the active preset of
// the app, and that a change of the effect of a built-in preset leaves toxics
// no preset holds.
func TestRecord(t *testing.T) {
	slow := DefaultPresets()[1]
	off := toxicWith(100)
	off.Disabled = true
	changed := Set{latencyDown: toxicWith(1, 0)}
	for name, tc := range map[string]struct {
		active     string
		global     Set
		want       string
		wantPreset Set
	}{
		"app preset":               {active: "Slow 4G", global: changed, want: "Slow 4G", wantPreset: changed},
		"no throttling changed":    {active: "No throttling", global: changed},
		"no throttling, toxic off": {active: "No throttling", global: Set{latencyDown: off}, want: "No throttling"},
		"unreachable changed":      {active: "Unreachable", global: Set{}},
		"unsaved":                  {global: changed},
		"deleted preset":           {active: "Train", global: changed},
	} {
		t.Run(name, func(t *testing.T) {
			s := Settings{
				Global: tc.global, Endpoints: map[string]Set{},
				Presets: []Preset{slow}, Active: tc.active,
			}
			s.record()
			require.Equal(t, tc.want, s.Active)
			want := slow.Toxics
			if tc.wantPreset != nil {
				want = tc.wantPreset
			}
			require.Equal(t, want, s.Presets[0].Toxics)
		})
	}
}

// TestPresetCommands tests applying, creating and deleting presets, built-in
// ones included.
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
	for _, name := range []string{"Unreachable", "No throttling"} {
		require.NoError(t, s.ApplyPreset(name))
		p, err := s.Preset(name)
		require.NoError(t, err)
		require.Equal(t, p.Toxics, s.Global)
		require.Equal(t, name, s.Active)
	}

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
	require.Equal(t,
		toxicWith(5, 0),
		s.Presets[3].Endpoints["POST /upload/{$}"][latencyUp],
		"the preset holds a copy of the overrides")

	require.NoError(t, s.CreatePreset("Train", DefaultPresets()[1].Toxics, nil))
	require.Equal(t, "Train", s.Active, "the copy, not Slow 4G")
	require.NoError(t, s.ApplyPreset("Tiny"))
	require.Equal(t,
		map[string]Set{"POST /upload/{$}": {latencyUp: toxicWith(5, 0)}}, s.Endpoints)
	require.NoError(t, s.ApplyPreset("Train"))

	require.ErrorIs(t, s.CreatePreset("Fast 4G", Set{}, nil), ErrPresetExists)
	require.ErrorIs(t, s.CreatePreset("Unreachable", Set{}, nil), ErrPresetExists)
	require.ErrorIs(t, s.CreatePreset(" ", Set{}, nil), ErrPresetUnnamed)
	require.Len(t, s.Presets, 5)

	require.NoError(t, s.DeletePreset("Train"))
	require.Equal(t, "No throttling", s.Active, "No throttling replaces the deleted preset")
	require.Empty(t, s.Global)
	require.NoError(t, s.DeletePreset("Fast 4G"))
	require.Equal(t, "Slow 4G", s.Presets[0].Name)
	require.ErrorIs(t, s.DeletePreset("Fast 4G"), ErrNoPreset)
	require.ErrorIs(t, s.DeletePreset("No throttling"), ErrPresetBuiltin)
	require.ErrorIs(t, s.DeletePreset("Unreachable"), ErrPresetBuiltin)

	p, err := DefaultPreset("3G")
	require.NoError(t, err)
	require.Equal(t, DefaultPresets()[2], p)
	_, err = DefaultPreset("Unreachable")
	require.ErrorIs(t, err, ErrNoPreset)
}

// TestScaffoldPresets tests that the datapages.yaml of init holds
// [DefaultPresets] in the form the settings page saves, and that watch starts
// with No throttling.
func TestScaffoldPresets(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, config.WriteDefault(dir))
	c, _, err := config.Load(dir)
	require.NoError(t, err)
	s := SettingsFromConfig(c.Watch.Toxiproxy)
	require.Equal(t, DefaultPresets(), s.Presets)
	require.Empty(t, s.Global)
	require.Equal(t, "No throttling", s.Active)
	require.True(t, strings.HasSuffix(config.DefaultYAML, "  toxiproxy:\n"+s.YAML(4, nil)),
		"the settings page writes:\n%s", s.YAML(4, nil))
}

// TestPresetsYAML tests that settings without presets write none.
func TestPresetsYAML(t *testing.T) {
	s := Settings{Global: Set{}, Endpoints: map[string]Set{}}
	require.Empty(t, s.YAML(0, nil))
}
