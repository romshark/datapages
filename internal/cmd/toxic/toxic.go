// Package toxic implements the Toxiproxy front of watch mode and its toxic
// settings. The front passes each request to the dev server through the
// Toxiproxy proxy of its endpoint. Package
// [github.com/romshark/datapages/internal/cmd/toxic/ui] serves the settings page.
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
	// Downstream is the response.
	Downstream Stream = iota
	// Upstream is the request.
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

// Type is a toxic type. [Front] applies [ClientBandwidth], [ServerBandwidth],
// [DataCap] and [HTTPError]. Toxiproxy applies the rest.
type Type int8

const (
	Latency Type = iota
	ClientBandwidth
	ServerBandwidth
	DataCap
	SlowClose
	Timeout
	ResetPeer
	Slicer
	LimitData
	HTTPError
)

// Unit is the unit of a field value, the one Toxiproxy takes.
type Unit int8

const (
	UnitMillis Unit = iota
	UnitMicros
	UnitBps // bytes per second
	UnitBytes
	UnitStatus // an HTTP status code
	UnitChoice // the index of 1 of [Field.Choices]
)

// String returns the symbol of u, as in "ms".
func (u Unit) String() string {
	switch u {
	case UnitMillis:
		return "ms"
	case UnitMicros:
		return "µs"
	case UnitBps:
		return "B/s"
	case UnitStatus, UnitChoice:
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

	// Initial is the value the settings page turns the toxic on with.
	Initial int64

	// Choices maps a [UnitChoice] index to its datapages.yaml value.
	// The first value is the omitted default.
	Choices []string
}

// TypeInfo describes a [Type] for the settings page, datapages.yaml and Toxiproxy.
type TypeInfo struct {
	Name   string // in the Toxiproxy API and in the name of a [Slot]
	Key    string // in datapages.yaml
	Title  string
	Doc    string
	Fields []Field

	// Front reports whether the [Front] applies the type, not Toxiproxy.
	Front bool
}

// MaxFields is the largest field count of a toxic type.
const MaxFields = 3

// [config.MaxToxicDuration] in the units of the duration fields.
const (
	maxMillis = int64(config.MaxToxicDuration / time.Millisecond)
	maxMicros = int64(config.MaxToxicDuration / time.Microsecond)
)

// PerClient and PerRequest select how [ClientBandwidth] and [DataCap] are shared.
const (
	// PerClient shares the toxic across a browser's requests.
	PerClient int64 = iota
	// PerRequest gives each request an independent toxic.
	PerRequest
)

var perField = Field{
	Key: "per", Label: "Shared by", Unit: UnitChoice, Max: PerRequest,
	Choices: []string{"client", "request"},
}

// Types is indexed by [Type]. The Doc of a Toxiproxy type paraphrases
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
				Unit: UnitMillis, Max: maxMillis, Initial: 300,
			},
			{
				Key: "jitter", Attr: "jitter", Label: "Jitter",
				Unit: UnitMillis, Max: maxMillis,
			},
		},
	},
	ClientBandwidth: {
		Name: "client_bandwidth", Key: "client-bandwidth", Title: "Client bandwidth",
		Doc: "Limits each browser's data rate to simulate a slow network.",
		Fields: []Field{
			{
				Key: "rate", Label: "Rate",
				Unit: UnitBps, Min: 1, Max: config.MaxBandwidthRate, Initial: 64_000,
			},
			perField,
		},
		Front: true,
	},
	ServerBandwidth: {
		Name: "server_bandwidth", Key: "server-bandwidth", Title: "Server bandwidth",
		Doc: "Limits the combined data rate of all browsers.",
		Fields: []Field{
			{
				Key: "rate", Label: "Rate",
				Unit: UnitBps, Min: 1, Max: config.MaxBandwidthRate, Initial: 1_000_000,
			},
		},
		Front: true,
	},
	DataCap: {
		Name: "data_cap", Key: "data-cap", Title: "Data cap",
		Doc: "Limits the data rate after a client or request uses its volume.",
		Fields: []Field{
			{Key: "volume", Label: "Volume", Unit: UnitBytes, Initial: 1 << 20},
			{
				Key: "rate", Label: "Throttled rate",
				Unit: UnitBps, Min: 1, Max: config.MaxBandwidthRate, Initial: 16_000,
			},
			perField,
		},
		Front: true,
	},
	SlowClose: {
		Name: "slow_close", Key: "slow-close", Title: "Slow close",
		Doc: "Delays closing the connection.",
		Fields: []Field{
			{
				Key: "delay", Attr: "delay", Label: "Delay",
				Unit: UnitMillis, Max: maxMillis, Initial: 1000,
			},
		},
	},
	Timeout: {
		Name: "timeout", Key: "timeout", Title: "Timeout",
		Doc: "Stops all data and closes the connection after the timeout. " +
			"0 leaves it open until the browser gives up.",
		Fields: []Field{
			{
				Key: "timeout", Attr: "timeout", Label: "Timeout",
				Unit: UnitMillis, Max: maxMillis, Initial: 5000,
			},
		},
	},
	ResetPeer: {
		Name: "reset_peer", Key: "reset-peer", Title: "Reset peer",
		Doc: "Resets the connection once data arrives, after the timeout.",
		Fields: []Field{
			{
				Key: "timeout", Attr: "timeout", Label: "Timeout",
				Unit: UnitMillis, Max: maxMillis,
			},
		},
	},
	Slicer: {
		Name: "slicer", Key: "slicer", Title: "Slicer",
		Doc: "Splits data into delayed chunks " +
			"with the configured average size and variation.",
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
				Unit: UnitMicros, Max: maxMicros, Initial: 10000,
			},
		},
	},
	LimitData: {
		Name: "limit_data", Key: "limit-data", Title: "Limit data",
		Doc: "Closes the connection after the byte limit.",
		Fields: []Field{
			{Key: "bytes", Attr: "bytes", Label: "Bytes", Unit: UnitBytes, Initial: 1024},
		},
	},
	HTTPError: {
		Name: "http_error", Key: "http-error", Title: "HTTP error",
		Doc: "Answers with the status instead of the app.",
		Fields: []Field{
			{
				Key: "status", Label: "Status",
				Unit: UnitStatus, Min: 400, Max: 599, Initial: 500,
			},
		},
		Front: true,
	},
}

// Slot is the type and stream of 1 toxic of a scope. Its name is the default
// name Toxiproxy gives the toxic, as in "latency_downstream".
type Slot struct {
	Type   Type
	Stream Stream
}

// Name returns the name of s, as in "latency_downstream".
func (s Slot) Name() string { return Types[s.Type].Name + "_" + s.Stream.String() }

// MarshalText returns the name of s.
func (s Slot) MarshalText() ([]byte, error) { return []byte(s.Name()), nil }

// UnmarshalText parses the name of a slot.
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

	// Toxicity is the probability from 0 to 1 that the toxic applies to a request.
	Toxicity float32

	// Values holds the values of the fields of the type, in their order and units.
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

// Validate reports a value of x that Toxiproxy rejects or cannot take.
// The rules match those of [config.Toxics].
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

// Set holds the toxics of 1 scope by slot. A slot missing from the global
// scope has no toxic. A slot missing on an endpoint inherits the global toxic.
type Set map[Slot]Toxic

// Settings holds the presets and the toxics in effect.
type Settings struct {
	// Global holds the toxics in effect, those of the preset Active.
	Global Set

	// Endpoints maps the route pattern of an endpoint to its overrides.
	Endpoints map[string]Set

	// Presets are in the order the settings page lists them.
	Presets []Preset

	// Active is the name of the preset of [Settings.AllPresets] in effect.
	// Empty means a changed built-in preset, which no preset holds yet.
	// datapages.yaml does not hold it, and watch starts with No throttling.
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

// cloneEndpoints returns a non-nil deep copy of e.
func cloneEndpoints(e map[string]Set) map[string]Set {
	c := make(map[string]Set, len(e))
	for k, v := range e {
		c[k] = maps.Clone(v)
	}
	return c
}

// active is 1 toxic that applies.
type active struct {
	slot  Slot
	toxic Toxic
}

// effective returns the toxics that apply to an endpoint with the overrides o,
// those of the [Front] when front is set, else those of Toxiproxy. An override
// replaces the global toxic of its slot. A disabled toxic is left out.
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

// attributes returns the Toxiproxy attributes of a.
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

// SettingsFromConfig returns the presets c configures with No throttling in effect.
// It adds no [DefaultPresets] to those of c.
func SettingsFromConfig(c *config.WatchToxiproxy) Settings {
	s := Settings{
		Global: Set{}, Endpoints: map[string]Set{}, Active: config.PresetNoThrottling,
	}
	if c == nil {
		return s
	}
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

func endpointsFromConfig(
	c map[config.EndpointPattern]config.Toxics, global Set,
) map[string]Set {
	e := make(map[string]Set, len(c))
	for p, t := range c {
		e[string(p)] = setFromConfig(t, global)
	}
	return e
}

// setFromConfig returns the toxics c configures. A disabled toxic with invalid
// values takes those of its slot in global, else those of [NewToxic], which
// the settings page turns it on with.
func setFromConfig(c config.Toxics, global Set) Set {
	s := Set{}
	addFromConfig(s, global, Latency, c.Latency,
		func(t config.ToxicLatency) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{
				t.Latency.Milliseconds(),
				t.Jitter.Milliseconds(),
			}
		})
	addFromConfig(s, global, ClientBandwidth, c.ClientBandwidth,
		func(t config.ToxicClientBandwidth) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Rate, perFromConfig(t.Per)}
		})
	addFromConfig(s, global, ServerBandwidth, c.ServerBandwidth,
		func(t config.ToxicServerBandwidth) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Rate}
		})
	addFromConfig(s, global, DataCap, c.DataCap,
		func(t config.ToxicDataCap) (config.ToxicBase, []int64) {
			return t.ToxicBase, []int64{t.Volume, t.Rate, perFromConfig(t.Per)}
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

func perFromConfig(p config.ToxicPer) int64 {
	if p == config.ToxicPerRequest {
		return PerRequest
	}
	return PerClient
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

// YAML returns the presets of s as the presets key of watch.toxiproxy,
// indented by indent spaces. Endpoints follow the order of patterns, unknown
// ones come last.
func (s Settings) YAML(indent int, patterns []string) string {
	var b strings.Builder
	presetsYAML(&b, indent, s.Presets, patterns)
	return b.String()
}

// toxicsYAML writes the toxics and endpoints keys of a preset.
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
				if f.Unit == UnitChoice && x.Values[i] == 0 {
					continue
				}
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

// writeLine writes parts and a newline to b without joining them first.
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
	case UnitChoice:
		return f.Choices[v]
	}
	return strconv.FormatInt(v, 10)
}
