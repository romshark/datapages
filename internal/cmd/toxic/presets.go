package toxic

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/romshark/datapages/internal/cmd/config"
)

// Preset is a named set of global toxics and endpoint overrides.
type Preset struct {
	Name   string
	Toxics Set

	// Endpoints maps the route pattern of an endpoint to its overrides. Not nil.
	Endpoints map[string]Set
}

// DefaultPresets returns the presets of [config.DefaultYAML]. They are the
// throttling presets of Chrome DevTools in the order of its menu, from
// [NetworkManager.ts] and [ThrottlingPresets.ts], rounded to whole ms and
// KB/s of 1000 bytes. Chrome delays each response by the latency, as the
// downstream latency toxic does. Chrome shares the bandwidth among the
// requests of a tab, the [Front] among those of a browser profile.
//
// [NetworkManager.ts]: https://github.com/ChromeDevTools/devtools-frontend/blob/main/front_end/core/sdk/NetworkManager.ts
// [ThrottlingPresets.ts]: https://github.com/ChromeDevTools/devtools-frontend/blob/main/front_end/panels/mobile_throttling/ThrottlingPresets.ts
func DefaultPresets() []Preset {
	throttled := func(name string, latency, down, up int64) Preset {
		return Preset{Name: name, Endpoints: map[string]Set{}, Toxics: Set{
			{
				Type: Latency, Stream: Downstream,
			}: {
				Toxicity: 1, Values: [MaxFields]int64{latency},
			},
			{
				Type: ClientBandwidth, Stream: Downstream,
			}: {
				Toxicity: 1, Values: [MaxFields]int64{down},
			},
			{
				Type: ClientBandwidth, Stream: Upstream,
			}: {
				Toxicity: 1, Values: [MaxFields]int64{up},
			},
		}}
	}
	return []Preset{
		// 60 ms * 2.75, 9 Mbps * 0.9 down, 1.5 Mbps * 0.9 up.
		throttled("Fast 4G", 165, 1013, 169),
		// 150 ms * 3.75, 1.6 Mbps * 0.9 down, 750 Kbps * 0.9 up.
		throttled("Slow 4G", 563, 180, 84),
		// 400 ms * 5, 500 Kbps * 0.8 down and up.
		throttled("3G", 2000, 50, 50),
	}
}

// DefaultPreset returns the preset of [DefaultPresets] with the name.
func DefaultPreset(name string) (Preset, error) {
	return findPreset(DefaultPresets(), name)
}

// AllPresets returns No throttling, the presets of s and Unreachable.
// datapages.yaml cannot hold the 2 built-in presets.
func (s Settings) AllPresets() []Preset {
	return slices.Concat([]Preset{noThrottling()}, s.Presets, []Preset{unreachable()})
}

// noThrottling returns the preset No throttling, which has no toxics.
func noThrottling() Preset {
	return Preset{
		Name: config.PresetNoThrottling, Toxics: Set{}, Endpoints: map[string]Set{},
	}
}

// unreachable returns the preset Unreachable, which resets every connection
// while the browser stays online. The upstream reset ends a request before the
// app gets it, the downstream one ends an open stream at its next event.
func unreachable() Preset {
	return Preset{Name: config.PresetUnreachable, Endpoints: map[string]Set{}, Toxics: Set{
		{Type: ResetPeer, Stream: Upstream}:   {Toxicity: 1},
		{Type: ResetPeer, Stream: Downstream}: {Toxicity: 1},
	}}
}

// IsBuiltinPreset reports whether [Settings.AllPresets] adds the preset name.
func IsBuiltinPreset(name string) bool {
	return name == config.PresetNoThrottling || name == config.PresetUnreachable
}

// Preset returns the preset of [Settings.AllPresets] with the name.
func (s Settings) Preset(name string) (Preset, error) {
	return findPreset(s.AllPresets(), name)
}

func findPreset(presets []Preset, name string) (Preset, error) {
	i := slices.IndexFunc(presets, func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, fmt.Errorf("%w %q", ErrNoPreset, name)
	}
	return presets[i], nil
}

// record keeps a change of the toxics in effect. A preset of the app takes
// it. A built-in preset ends when its effect changes, which leaves toxics no
// preset holds.
func (s *Settings) record() {
	if s.Active == "" {
		return
	}
	if IsBuiltinPreset(s.Active) {
		p, _ := s.Preset(s.Active)
		if !sameEffect(s.Global, s.Endpoints, p.Toxics, p.Endpoints) {
			s.Active = ""
		}
		return
	}
	i := slices.IndexFunc(s.Presets, func(p Preset) bool { return p.Name == s.Active })
	if i < 0 {
		s.Active = ""
		return
	}
	s.Presets[i].Toxics = maps.Clone(s.Global)
	s.Presets[i].Endpoints = cloneEndpoints(s.Endpoints)
}

// sameEffect reports whether the global toxics g1 with the overrides e1 apply
// the same toxics to every endpoint as g2 with e2. A disabled toxic counts as
// none, and an override that changes nothing as no override.
func sameEffect(g1 Set, e1 map[string]Set, g2 Set, e2 map[string]Set) bool {
	if !slices.Equal(applied(g1, nil), applied(g2, nil)) {
		return false
	}
	for _, e := range []map[string]Set{e1, e2} {
		for p := range e {
			if !slices.Equal(applied(g1, e1[p]), applied(g2, e2[p])) {
				return false
			}
		}
	}
	return true
}

// applied returns the toxics that apply to an endpoint with the overrides o,
// those of Toxiproxy first.
func applied(global, o Set) []active {
	return append(effective(global, o, false), effective(global, o, true)...)
}

var (
	// ErrNoPreset reports a name no preset has.
	ErrNoPreset = errors.New("no preset has the name")
	// ErrPresetExists reports a new preset name that a preset has.
	ErrPresetExists = errors.New("a preset has the name")
	// ErrPresetUnnamed reports a new preset without a name.
	ErrPresetUnnamed = errors.New("the preset has no name")
	// ErrPresetBuiltin reports a built-in preset, which cannot be deleted.
	ErrPresetBuiltin = errors.New("the preset is built in")
)

// ApplyPreset replaces the toxics in effect with those of the preset with the
// name and makes it [Settings.Active].
func (s *Settings) ApplyPreset(name string) error {
	p, err := s.Preset(name)
	if err != nil {
		return err
	}
	s.Global = maps.Clone(p.Toxics)
	if s.Global == nil {
		s.Global = Set{}
	}
	s.Endpoints = cloneEndpoints(p.Endpoints)
	s.Active = name
	return nil
}

// CreatePreset appends a preset with the trimmed name and copies of global
// and endpoints, and applies it.
func (s *Settings) CreatePreset(
	name string, global Set, endpoints map[string]Set,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrPresetUnnamed
	}
	if _, err := s.Preset(name); err == nil {
		return fmt.Errorf("%w %q", ErrPresetExists, name)
	}
	p := Preset{Name: name, Toxics: Set{}, Endpoints: cloneEndpoints(endpoints)}
	maps.Copy(p.Toxics, global)
	s.Presets = append(s.Presets, p)
	return s.ApplyPreset(name)
}

// DeletePreset removes the preset with the name. When it is in effect,
// No throttling replaces it.
func (s *Settings) DeletePreset(name string) error {
	if IsBuiltinPreset(name) {
		return fmt.Errorf("%w %q", ErrPresetBuiltin, name)
	}
	i := slices.IndexFunc(s.Presets, func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return fmt.Errorf("%w %q", ErrNoPreset, name)
	}
	s.Presets = slices.Delete(s.Presets, i, i+1)
	if s.Active == name {
		return s.ApplyPreset(config.PresetNoThrottling)
	}
	return nil
}

// presetsYAML writes the presets key of [Settings.YAML].
func presetsYAML(b *strings.Builder, indent int, presets []Preset, patterns []string) {
	if len(presets) == 0 {
		return
	}
	pad := strings.Repeat(" ", indent)
	writeLine(b, pad, "presets:")
	for _, p := range presets {
		writeLine(b, pad, "  - name: ", strconv.Quote(p.Name))
		toxicsYAML(b, indent+4, p.Toxics, p.Endpoints, patterns)
	}
}
