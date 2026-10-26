// Package toxic provides the Toxiproxy front of watch mode: an HTTP server that
// passes each request through the Toxiproxy proxy of its endpoint to the dev
// server, and the toxic settings it applies. Package
// [github.com/romshark/datapages/internal/cmd/toxic/ui] provides the settings page.
package toxic

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/romshark/datapages/internal/cmd/config"
)

// Stream is the direction of the data a toxic acts on.
type Stream int8

const (
	// Downstream is the data the server sends, the response.
	Downstream Stream = iota
	// Upstream is the data the client sends, the request.
	Upstream
)

// Streams lists every [Stream] in the order the settings page shows them.
var Streams = [...]Stream{Downstream, Upstream}

// String returns the name Toxiproxy and datapages.yaml give s.
func (s Stream) String() string {
	if s == Upstream {
		return "upstream"
	}
	return "downstream"
}

// Type is 1 toxic type. Toxiproxy applies every type but [Bandwidth] and [HTTPError],
// which the [Front] applies.
type Type int8

const (
	Latency Type = iota
	Bandwidth
	SlowClose
	Timeout
	ResetPeer
	Slicer
	LimitData
	HTTPError
)

// Unit is what a field value counts. Each is the unit Toxiproxy takes.
type Unit int8

const (
	UnitMillis Unit = iota
	UnitMicros
	UnitKBps
	UnitBytes
	UnitStatus // an HTTP status code
)

// String returns the symbol of u, as in "ms".
func (u Unit) String() string {
	switch u {
	case UnitMillis:
		return "ms"
	case UnitMicros:
		return "µs"
	case UnitKBps:
		return "KB/s"
	case UnitStatus:
		return ""
	}
	return "B"
}

// Field is 1 attribute of a toxic type.
type Field struct {
	Key   string // in datapages.yaml
	Attr  string // in the Toxiproxy API
	Label string
	Unit  Unit
	Min   int64
	Max   int64 // 0 for none

	// Initial is the value of the field when a toxic is turned on
	// on the settings page.
	Initial int64
}

// TypeInfo describes a [Type] to the settings page, to datapages.yaml and to Toxiproxy.
type TypeInfo struct {
	Name   string // in the Toxiproxy API and in the name of a [Slot]
	Key    string // in datapages.yaml
	Title  string
	Doc    string
	Fields []Field

	// Front reports whether the [Front] applies the type, not Toxiproxy.
	Front bool
}

// MaxFields is the most fields a toxic type has, the 3 of [Slicer].
const MaxFields = 3

// Types is indexed by [Type]. The docs of the types of Toxiproxy paraphrase
// [the Toxiproxy toxics] for an HTTP request.
//
// [the Toxiproxy toxics]: https://github.com/Shopify/toxiproxy#toxics
var Types = [...]TypeInfo{
	Latency: {
		Name: "latency", Key: "latency", Title: "Latency",
		Doc: "Delays the data by the latency, plus or minus up to the jitter.",
		Fields: []Field{
			{
				Key: "latency", Attr: "latency", Label: "Latency",
				Unit: UnitMillis, Initial: 300,
			},
			{Key: "jitter", Attr: "jitter", Label: "Jitter", Unit: UnitMillis},
		},
	},
	Bandwidth: {
		Name: "bandwidth", Key: "bandwidth", Title: "Bandwidth",
		Doc: "Limits the rate of the data, which the requests of 1 browser share.",
		Fields: []Field{
			{
				Key: "rate", Label: "Rate",
				Unit: UnitKBps, Min: 1, Initial: 64,
			},
		},
		Front: true,
	},
	SlowClose: {
		Name: "slow_close", Key: "slow-close", Title: "Slow close",
		Doc: "Delays closing the connection.",
		Fields: []Field{
			{
				Key: "delay", Attr: "delay", Label: "Delay",
				Unit: UnitMillis, Initial: 1000,
			},
		},
	},
	Timeout: {
		Name: "timeout", Key: "timeout", Title: "Timeout",
		Doc: "Stops all data and closes the connection after the timeout. " +
			"0 holds it open until the browser gives up.",
		Fields: []Field{
			{
				Key: "timeout", Attr: "timeout", Label: "Timeout",
				Unit: UnitMillis, Initial: 5000,
			},
		},
	},
	ResetPeer: {
		Name: "reset_peer", Key: "reset-peer", Title: "Reset peer",
		Doc: "Resets the connection once data arrives, after the timeout.",
		Fields: []Field{
			{Key: "timeout", Attr: "timeout", Label: "Timeout", Unit: UnitMillis},
		},
	},
	Slicer: {
		Name: "slicer", Key: "slicer", Title: "Slicer",
		Doc: "Splits the data into chunks of the average size, plus or minus " +
			"up to the variation, and delays each chunk.",
		Fields: []Field{
			{
				Key: "average-size", Attr: "average_size", Label: "Average size",
				Unit: UnitBytes, Min: 1, Initial: 64,
			},
			{
				Key: "size-variation", Attr: "size_variation", Label: "Variation",
				Unit: UnitBytes, Initial: 32,
			},
			{
				Key: "delay", Attr: "delay", Label: "Delay",
				Unit: UnitMicros, Initial: 10000,
			},
		},
	},
	LimitData: {
		Name: "limit_data", Key: "limit-data", Title: "Limit data",
		Doc: "Closes the connection after the number of bytes.",
		Fields: []Field{
			{Key: "bytes", Attr: "bytes", Label: "Bytes", Unit: UnitBytes, Initial: 1024},
		},
	},
	HTTPError: {
		Name: "http_error", Key: "http-error", Title: "HTTP error",
		Doc: "Answers with the status. On the request, the app never gets it. " +
			"On the response, the app answers and the browser gets the status instead.",
		Fields: []Field{
			{
				Key: "status", Label: "Status",
				Unit: UnitStatus, Min: 400, Max: 599, Initial: 500,
			},
		},
		Front: true,
	},
}

// Slot names 1 toxic of a scope: its type and the stream it acts on.
// Toxiproxy names it the same way by default, as in "latency_downstream".
type Slot struct {
	Type   Type
	Stream Stream
}

// Name returns the name of s, as in "latency_downstream".
func (s Slot) Name() string { return Types[s.Type].Name + "_" + s.Stream.String() }

// MarshalText returns the name of s, which [Slot.UnmarshalText] reads.
func (s Slot) MarshalText() ([]byte, error) { return []byte(s.Name()), nil }

// UnmarshalText reads the name of a slot, as in "latency_downstream".
func (s *Slot) UnmarshalText(text []byte) error {
	for _, x := range Slots {
		if x.Name() == string(text) {
			*s = x
			return nil
		}
	}
	return fmt.Errorf("unknown toxic slot %q", text)
}

// Slots lists every [Slot] in the order the settings page and the YAML show them.
var Slots = func() []Slot {
	s := make([]Slot, 0, len(Types)*len(Streams))
	for t := range Types {
		for _, st := range Streams {
			s = append(s, Slot{Type: Type(t), Stream: st})
		}
	}
	return s
}()

// Toxic is the setting of 1 [Slot] of a scope.
type Toxic struct {
	// Disabled turns the toxic off. On an endpoint it turns the global toxic off.
	Disabled bool

	// Toxicity is the probability from 0 to 1 that the toxic applies
	// to a connection.
	Toxicity float32

	// Values holds the field values of the type in the order of its fields,
	// in the unit of each.
	Values [MaxFields]int64
}

// NewToxic returns the toxic the settings page turns on for a slot of type t.
func NewToxic(t Type) Toxic {
	x := Toxic{Toxicity: 1}
	for i, f := range Types[t].Fields {
		x.Values[i] = f.Initial
	}
	return x
}

// Validate reports a value of x Toxiproxy rejects or cannot take.
// The rules match those of datapages.yaml in [config.Toxics].
func (x Toxic) Validate(t Type) error {
	if x.Toxicity < 0 || x.Toxicity > 1 {
		return fmt.Errorf("toxicity %v must be from 0 to 1", x.Toxicity)
	}
	for i, f := range Types[t].Fields {
		if x.Values[i] < f.Min {
			return fmt.Errorf("%s must be at least %d", strings.ToLower(f.Label), f.Min)
		}
		if f.Max != 0 && x.Values[i] > f.Max {
			return fmt.Errorf("%s must be at most %d", strings.ToLower(f.Label), f.Max)
		}
	}
	if t == Slicer && x.Values[1] > x.Values[0] {
		return fmt.Errorf("variation must be at most the average size")
	}
	return nil
}

// Set holds the toxics of 1 scope by slot. In the global scope a missing slot
// has no toxic, on an endpoint it inherits the global toxic.
type Set map[Slot]Toxic

// Settings are the toxics of every scope.
type Settings struct {
	Global Set

	// Endpoints maps the route pattern of an endpoint to its overrides.
	Endpoints map[string]Set

	// Presets are in the order the settings page lists them.
	Presets []Preset

	// Active is the name of the preset applied last, which
	// [Settings.ActivePreset] prefers. datapages.yaml does not hold it.
	Active string
}

// Clone returns a deep copy of s whose maps are not nil.
func (s Settings) Clone() Settings {
	c := Settings{
		Global:    Set{},
		Endpoints: cloneEndpoints(s.Endpoints),
		Active:    s.Active,
	}
	maps.Copy(c.Global, s.Global)
	if s.Presets != nil {
		c.Presets = make([]Preset, len(s.Presets))
		for i, p := range s.Presets {
			c.Presets[i] = Preset{
				Name: p.Name, Toxics: Set{}, Endpoints: cloneEndpoints(p.Endpoints),
			}
			maps.Copy(c.Presets[i].Toxics, p.Toxics)
		}
	}
	return c
}

// cloneEndpoints returns a deep copy of the overrides e, which is not nil.
func cloneEndpoints(e map[string]Set) map[string]Set {
	c := make(map[string]Set, len(e))
	for k, v := range e {
		c[k] = maps.Clone(v)
	}
	return c
}

// overridesProxy reports whether s has a toxic of a type Toxiproxy applies.
func (s Set) overridesProxy() bool {
	for slot := range s {
		if !Types[slot.Type].Front {
			return true
		}
	}
	return false
}

// active is 1 toxic that applies.
type active struct {
	slot  Slot
	toxic Toxic
}

// effective returns the toxics of an endpoint with the overrides o:
// the toxic o has for a slot, else the global one. A disabled toxic is left out.
// It returns those the [Front] applies when front is set, else those Toxiproxy applies.
func effective(global, o Set, front bool) []active {
	var a []active
	for _, s := range Slots {
		if Types[s.Type].Front != front {
			continue
		}
		x, ok := o[s]
		if !ok {
			x, ok = global[s]
		}
		if ok && !x.Disabled {
			a = append(a, active{slot: s, toxic: x})
		}
	}
	return a
}

// attributes returns the attributes Toxiproxy takes for x.
func (a active) attributes() map[string]int64 {
	attrs := make(map[string]int64, MaxFields)
	for i, f := range Types[a.slot.Type].Fields {
		attrs[f.Attr] = a.toxic.Values[i]
	}
	return attrs
}

// createJSON returns the body Toxiproxy takes to create the toxic.
func (a active) createJSON() []byte {
	b, _ := json.Marshal(struct {
		Name       string           `json:"name"`
		Type       string           `json:"type"`
		Stream     string           `json:"stream"`
		Toxicity   float32          `json:"toxicity"`
		Attributes map[string]int64 `json:"attributes"`
	}{
		Name:       a.slot.Name(),
		Type:       Types[a.slot.Type].Name,
		Stream:     a.slot.Stream.String(),
		Toxicity:   a.toxic.Toxicity,
		Attributes: a.attributes(),
	})
	return b
}

// updateJSON returns the body Toxiproxy takes to update the toxic.
func (a active) updateJSON() []byte {
	b, _ := json.Marshal(struct {
		Toxicity   float32          `json:"toxicity"`
		Attributes map[string]int64 `json:"attributes"`
	}{
		Toxicity:   a.toxic.Toxicity,
		Attributes: a.attributes(),
	})
	return b
}

// SettingsFromConfig returns the settings c configures.
// The presets are those of c only, none without.
func SettingsFromConfig(c *config.WatchToxiproxy) Settings {
	s := Settings{Global: Set{}, Endpoints: map[string]Set{}}
	if c == nil {
		return s
	}
	s.Global = setFromConfig(c.Toxics, nil)
	s.Endpoints = endpointsFromConfig(c.Endpoints, s.Global)
	for _, p := range c.Presets {
		g := setFromConfig(p.Toxics, nil)
		s.Presets = append(s.Presets,
			Preset{
				Name:      p.Name,
				Toxics:    g,
				Endpoints: endpointsFromConfig(p.Endpoints, g),
			})
	}
	return s
}

// endpointsFromConfig returns the overrides c configures with the global
// toxics global.
func endpointsFromConfig(
	c map[config.EndpointPattern]config.Toxics, global Set,
) map[string]Set {
	e := make(map[string]Set, len(c))
	for p, t := range c {
		e[string(p)] = setFromConfig(t, global)
	}
	return e
}

// setFromConfig returns the toxics c configures. A disabled toxic without
// valid values gets those of the toxic of its slot in global,
// else those of [NewToxic], for the settings page to turn it on with.
func setFromConfig(c config.Toxics, global Set) Set {
	s := Set{}
	addFromConfig(s, global, Latency, c.Latency,
		func(t config.ToxicLatency) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{
				t.Latency.Milliseconds(),
				t.Jitter.Milliseconds(),
			}
		})
	addFromConfig(s, global, Bandwidth, c.Bandwidth,
		func(t config.ToxicBandwidth) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Rate}
		})
	addFromConfig(s, global, SlowClose, c.SlowClose,
		func(t config.ToxicSlowClose) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Delay.Milliseconds()}
		})
	addFromConfig(s, global, Timeout, c.Timeout,
		func(t config.ToxicTimeout) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Timeout.Milliseconds()}
		})
	addFromConfig(s, global, ResetPeer, c.ResetPeer,
		func(t config.ToxicResetPeer) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Timeout.Milliseconds()}
		})
	addFromConfig(s, global, Slicer, c.Slicer,
		func(t config.ToxicSlicer) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{
				t.AverageSize, t.SizeVariation, t.Delay.Microseconds(),
			}
		})
	addFromConfig(s, global, LimitData, c.LimitData,
		func(t config.ToxicLimitData) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Bytes}
		})
	addFromConfig(s, global, HTTPError, c.HTTPError,
		func(t config.ToxicHTTPError) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Status}
		})
	return s
}

func addFromConfig[T any](
	s, global Set, t Type, c *config.ToxicStreams[T],
	values func(T) (config.ToxicBase, []int64),
) {
	if c == nil {
		return
	}
	for _, st := range Streams {
		v := c.Downstream
		if st == Upstream {
			v = c.Upstream
		}
		if v == nil {
			continue
		}
		base, vals := values(*v)
		x := Toxic{Disabled: base.Disabled, Toxicity: 1}
		if base.Toxicity != nil {
			x.Toxicity = *base.Toxicity
		}
		copy(x.Values[:], vals)
		slot := Slot{Type: t, Stream: st}
		if x.Disabled && x.Validate(t) != nil {
			fill, ok := global[slot]
			if !ok {
				fill = NewToxic(t)
			}
			x.Values = fill.Values
		}
		s[slot] = x
	}
}

// YAML returns s as the toxics, endpoints and presets of watch.toxiproxy in
// datapages.yaml, indented by indent spaces. The endpoints are listed in the
// order of patterns, the overrides of an unknown endpoint after the others.
func (s Settings) YAML(indent int, patterns []string) string {
	var b strings.Builder
	toxicsYAML(&b, indent, s.Global, s.Endpoints, patterns)
	presetsYAML(&b, indent, s.Presets, patterns)
	return b.String()
}

// toxicsYAML writes the global toxics and the overrides of the endpoints as
// the toxics and endpoints keys of datapages.yaml, indented by indent spaces.
// The endpoints are listed in the order of patterns, the overrides of an
// unknown endpoint after the others.
func toxicsYAML(
	b *strings.Builder,
	indent int,
	global Set,
	endpoints map[string]Set,
	patterns []string,
) {
	pad := strings.Repeat(" ", indent)
	if len(global) > 0 {
		writeLine(b, pad, "toxics:")
		writeSetYAML(b, indent+2, global)
	}
	var keys []string
	for _, p := range patterns {
		if len(endpoints[p]) > 0 {
			keys = append(keys, p)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(endpoints)) {
		if len(endpoints[p]) > 0 && !slices.Contains(patterns, p) {
			keys = append(keys, p)
		}
	}
	if len(keys) > 0 {
		writeLine(b, pad, "endpoints:")
		for _, p := range keys {
			writeLine(b, pad, "  ", strconv.Quote(p), ":")
			writeSetYAML(b, indent+4, endpoints[p])
		}
	}
}

func writeSetYAML(b *strings.Builder, indent int, s Set) {
	pad := strings.Repeat(" ", indent)
	for t, info := range Types {
		var wrote bool
		for _, st := range Streams {
			x, ok := s[Slot{Type: Type(t), Stream: st}]
			if !ok {
				continue
			}
			if !wrote {
				writeLine(b, pad, info.Key, ":")
				wrote = true
			}
			writeLine(b, pad, "  ", st.String(), ":")
			for i, f := range info.Fields {
				writeLine(b, pad, "    ", f.Key, ": ", f.yamlValue(x.Values[i]))
			}
			if x.Toxicity != 1 {
				writeLine(b, pad, "    toxicity: ",
					strconv.FormatFloat(float64(x.Toxicity), 'g', -1, 32))
			}
			if x.Disabled {
				writeLine(b, pad, "    disabled: true")
			}
		}
	}
}

// writeLine writes parts and a newline to b, without first joining them
// into a string of their own.
func writeLine(b *strings.Builder, parts ...string) {
	for _, p := range parts {
		b.WriteString(p)
	}
	b.WriteByte('\n')
}

// yamlValue renders v as datapages.yaml takes it for f.
func (f Field) yamlValue(v int64) string {
	switch f.Unit {
	case UnitMillis:
		return (time.Duration(v) * time.Millisecond).String()
	case UnitMicros:
		return (time.Duration(v) * time.Microsecond).String()
	}
	return strconv.FormatInt(v, 10)
}
