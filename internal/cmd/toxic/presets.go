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

// Preset is a named set of global toxics and endpoint overrides,
// which the settings page applies in 1 step.
type Preset struct {
	Name   string
	Toxics Set

	// Endpoints maps the route pattern of an endpoint to its overrides. Not nil.
	Endpoints map[string]Set
}

// DefaultPresets returns the presets of Datapages, which [config.DefaultYAML]
// holds and the dialog of a new preset copies from. They are the throttling
// presets of Chrome DevTools, in the order of its throttling menu,
// from [NetworkManager.ts] and [ThrottlingPresets.ts].
//
// Chrome delays each response by the latency, which the downstream latency
// toxic does too. Chrome shares the bandwidth among the requests of a tab,
// the [Front] among those of a browser profile. The toxics take whole milliseconds
// and whole KB/s of 1000 bytes, to which the values of Chrome are rounded.
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
				Type: Bandwidth, Stream: Downstream,
			}: {
				Toxicity: 1, Values: [MaxFields]int64{down},
			},
			{
				Type: Bandwidth, Stream: Upstream,
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

// AllPresets returns the presets the settings page lists: No throttling,
// which has no toxics, the presets of s, and Offline. Chrome DevTools lists
// those 2 apart from its throttling presets too, and datapages.yaml cannot hold them.
func (s Settings) AllPresets() []Preset {
	noThrottling := Preset{
		Name: config.PresetNoThrottling, Toxics: Set{}, Endpoints: map[string]Set{},
	}
	return slices.Concat([]Preset{noThrottling}, s.Presets, []Preset{offline()})
}

// offline returns the preset Offline. Chrome fails every request.
// The upstream reset ends a request before the app gets it,
// the downstream one an open stream at its next event.
func offline() Preset {
	return Preset{Name: config.PresetOffline, Endpoints: map[string]Set{}, Toxics: Set{
		{Type: ResetPeer, Stream: Upstream}:   {Toxicity: 1},
		{Type: ResetPeer, Stream: Downstream}: {Toxicity: 1},
	}}
}

// IsBuiltinPreset reports whether the preset with the name is one that
// [Settings.AllPresets] adds.
func IsBuiltinPreset(name string) bool {
	return name == config.PresetNoThrottling || name == config.PresetOffline
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

// ActivePreset returns the name of the preset of [Settings.AllPresets] whose
// toxics and overrides apply to every endpoint as those of s do, or "" for none.
// Of several, it returns [Settings.Active], else the first.
func (s Settings) ActivePreset() string {
	matches := func(p Preset) bool {
		return sameEffect(s.Global, s.Endpoints, p.Toxics, p.Endpoints)
	}
	all := s.AllPresets()
	if p, err := findPreset(all, s.Active); err == nil && matches(p) {
		return p.Name
	}
	if i := slices.IndexFunc(all, matches); i >= 0 {
		return all[i].Name
	}
	return ""
}

// sameEffect reports whether the global toxics g1 with the overrides e1 apply
// the same toxics to every endpoint as g2 with e2. A toxic turned off applies
// as none, and an override that changes nothing as no override.
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
// those Toxiproxy applies first.
func applied(global, o Set) []active {
	return append(effective(global, o, false), effective(global, o, true)...)
}

var (
	// ErrNoPreset is a preset name that no preset has.
	ErrNoPreset = errors.New("no preset has the name")
	// ErrPresetExists is the name of a new preset that a preset has.
	ErrPresetExists = errors.New("a preset has the name")
	// ErrPresetUnnamed is a new preset without a name.
	ErrPresetUnnamed = errors.New("the preset has no name")
	// ErrPresetBuiltin is the name of a preset [Settings.AllPresets] adds,
	// which cannot be deleted.
	ErrPresetBuiltin = errors.New("the preset is built in")
)

// ApplyPreset makes the global toxics and the overrides those of the preset
// of [Settings.AllPresets] with the name, and the preset [Settings.Active].
func (s *Settings) ApplyPreset(name string) error {
	p, err := s.Preset(name)
	if err != nil {
		return err
	}
	s.Global = Set{}
	maps.Copy(s.Global, p.Toxics)
	s.Endpoints = cloneEndpoints(p.Endpoints)
	s.Active = name
	return nil
}

// CreatePreset adds a preset with the name, without the spaces around it,
// and a copy of the global toxics global and the overrides endpoints after
// the others and applies it.
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

// DeletePreset removes the preset with the name. The global toxics stay.
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
		s.Active = ""
	}
	return nil
}

// presetsYAML writes presets as watch.toxiproxy.presets in datapages.yaml,
// indented by indent spaces, nothing for none.
// The endpoints of a preset are listed as [toxicsYAML] lists them.
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
