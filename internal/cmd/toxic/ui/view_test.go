package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/cmd/toxic"
)

var latencyDown = toxic.Slot{Type: toxic.Latency, Stream: toxic.Downstream}

func toxicWith(values ...int64) toxic.Toxic {
	x := toxic.Toxic{Toxicity: 1}
	copy(x.Values[:], values)
	return x
}

// TestSetSlot tests setting the mode of a slot. A toxic turned off keeps its
// values, and an endpoint without overrides leaves no empty entry.
func TestSetSlot(t *testing.T) {
	const ep = "GET /{$}"
	disabled := func(x toxic.Toxic) toxic.Toxic { x.Disabled = true; return x }
	globalOnly := func(g toxic.Set) toxic.Settings {
		return toxic.Settings{Global: g, Endpoints: map[string]toxic.Set{}}
	}
	for name, tc := range map[string]struct {
		before toxic.Settings
		scope  string
		mode   string
		toxic  toxic.Toxic
		want   toxic.Settings
	}{
		"global on": {
			before: globalOnly(toxic.Set{}),
			mode:   modeOn, toxic: toxicWith(5, 1),
			want: globalOnly(toxic.Set{latencyDown: toxicWith(5, 1)}),
		},
		"global off keeps values": {
			before: globalOnly(toxic.Set{latencyDown: toxicWith(5, 1)}),
			mode:   modeOff,
			want:   globalOnly(toxic.Set{latencyDown: disabled(toxicWith(5, 1))}),
		},
		"global off of nothing": {
			before: globalOnly(toxic.Set{}),
			mode:   modeOff,
			want: globalOnly(toxic.Set{
				latencyDown: disabled(toxic.NewToxic(toxic.Latency)),
			}),
		},
		"endpoint off copies global": {
			before: globalOnly(toxic.Set{latencyDown: toxicWith(5, 1)}),
			scope:  ep, mode: modeOff,
			want: toxic.Settings{
				Global: toxic.Set{latencyDown: toxicWith(5, 1)},
				Endpoints: map[string]toxic.Set{
					ep: {latencyDown: disabled(toxicWith(5, 1))},
				},
			},
		},
		"endpoint inherit removes the last override": {
			before: toxic.Settings{
				Global:    toxic.Set{},
				Endpoints: map[string]toxic.Set{ep: {latencyDown: toxicWith(5, 1)}},
			},
			scope: ep, mode: modeInherit,
			want: globalOnly(toxic.Set{}),
		},
		"global inherit does nothing": {
			before: globalOnly(toxic.Set{latencyDown: toxicWith(5, 1)}),
			mode:   modeInherit,
			want:   globalOnly(toxic.Set{latencyDown: toxicWith(5, 1)}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := tc.before.Clone()
			setSlot(&s, tc.scope, latencyDown, tc.mode, tc.toxic)
			require.Equal(t, tc.want, s)
		})
	}
}

// TestMatches tests that each search term matches as a case-insensitive
// subsequence, independent of the other terms.
func TestMatches(t *testing.T) {
	const text = "POST /messages/sendmessage/{$} PageMessages.POSTSendMessage"
	for query, want := range map[string]bool{
		"":             true,
		"  ":           true,
		"msg send":     true,
		"send msg":     true,
		"PST":          true,
		"{$}":          true,
		"messages put": false,
		"zz":           false,
	} {
		require.Equal(t, want, matches(query, text), "%q", query)
	}
}

// TestPathParts tests splitting route paths into literal text, wildcards and
// the end anchor.
func TestPathParts(t *testing.T) {
	require.Equal(t, []pathPart{
		{text: "/post/"},
		{text: "{slug}", class: "tp-route-wild"},
		{text: "/files/"},
		{text: "{rest...}", class: "tp-route-wild"},
	}, pathParts("/post/{slug}/files/{rest...}"))
	require.Equal(t, []pathPart{
		{text: "/"},
		{text: "{$}", class: "tp-route-end"},
	}, pathParts("/{$}"))
	require.Equal(t, []pathPart{{text: "/static/"}}, pathParts("/static/"))
	require.Equal(t, []pathPart{{text: "/odd/{"}}, pathParts("/odd/{"))
}

// TestFilter tests the kinds filter. Empty is every kind. "none", sent with no
// toggle pressed, is no kind.
func TestFilter(t *testing.T) {
	for _, tc := range []struct {
		kinds   string
		pressed string
		all     bool
	}{
		{kinds: "", pressed: "get,stream,action,file", all: true},
		{kinds: "file-stream-get-action", pressed: "get,stream,action,file", all: true},
		{kinds: "action-get", pressed: "get,action", all: false},
		{kinds: "none", pressed: "", all: false},
	} {
		f := filter{kinds: tc.kinds}
		require.Equal(t, tc.pressed, f.pressedKinds(), tc.kinds)
		require.Equal(t, tc.all, f.allKinds(), tc.kinds)
	}
	f := filter{search: "lim", kinds: toxic.KindGET}
	require.True(t, f.shows(toxic.KindGET, "GET /limited/{$}"))
	require.False(t, f.shows(toxic.KindAction, "POST /limited/{$}"))
	require.False(t, f.shows(toxic.KindGET, "GET /{$}"))
}

// TestStatusOptions tests that the combobox offers the statuses net/http names
// from 400 to 599, plus an unnamed current status.
func TestStatusOptions(t *testing.T) {
	f := toxic.Types[toxic.HTTPError].Fields[0]
	opts := statusOptions(f, 500)
	require.Equal(t, statusOption{value: "400", label: "400 Bad Request"}, opts[0])
	require.Equal(t,
		statusOption{value: "511", label: "511 Network Authentication Required"},
		opts[len(opts)-1])
	require.Contains(t,
		opts, statusOption{value: "503", label: "503 Service Unavailable"})
	unnamed := statusOption{value: "499", label: "499"}
	require.NotContains(t, opts, unnamed)
	require.Contains(t, statusOptions(f, 499), unnamed)
}

// TestSetField tests setting a field by key, the toxicity percentage,
// an empty field as 0, a byte field in a unit, and the errors.
func TestSetField(t *testing.T) {
	x := toxicWith(100, 50)
	require.NoError(t, setField(&x, toxic.Latency, "jitter", " 20 ", ""))
	require.NoError(t, setField(&x, toxic.Latency, "toxicity", "30", ""))
	require.NoError(t, setField(&x, toxic.Latency, "latency", "", ""))
	want := toxicWith(0, 20)
	want.Toxicity = 0.3
	require.Equal(t, want, x)

	require.EqualError(t, setField(&x, toxic.Latency, "jitter", "1.5", ""),
		`jitter: "1.5" is not a whole number`)
	require.EqualError(t, setField(&x, toxic.Latency, "toxicity", "101", ""),
		"requests affected must be a whole percentage from 0 to 100")
	require.ErrorIs(t, setField(&x, toxic.Latency, "rate", "1", ""), errUnknownField)

	y := toxicWith(1024)
	require.NoError(t, setField(&y, toxic.LimitData, "bytes", "1.5", "KiB"))
	require.Equal(t, toxicWith(1536), y)
	require.NoError(t, setField(&y, toxic.LimitData, "bytes", "64", ""))
	require.Equal(t, toxicWith(64), y)
	require.EqualError(t, setField(&y, toxic.LimitData, "bytes", "1.5", "B"),
		`bytes: "1.5" is not a whole number`)
	require.ErrorIs(t, setField(&y, toxic.LimitData, "bytes", "1", "TB"), errUnknownUnit)

	z := toxicWith(64)
	require.NoError(t, setField(&z, toxic.ClientBandwidth, "per", "request", ""))
	require.Equal(t, toxicWith(64, toxic.PerRequest), z)
	require.EqualError(t, setField(&z, toxic.ClientBandwidth, "per", "tab", ""),
		`shared by must be client or request, not "tab"`)
}

// TestDefaultUnit tests that a value shows in the unit that shows it in the
// fewest characters with up to 3 decimals, the larger of 2 that tie.
func TestDefaultUnit(t *testing.T) {
	for name, tc := range map[string]struct {
		units []valueUnit
		v     int64
		want  string
	}{
		"zero":             {byteUnits, 0, "B"},
		"below 1 KB":       {byteUnits, 999, "B"},
		"KB":               {byteUnits, 1000, "KB"},
		"shorter in B":     {byteUnits, 1023, "B"},
		"KiB":              {byteUnits, 1024, "KiB"},
		"half KiB":         {byteUnits, 1536, "KiB"},
		"KiB ties KB":      {byteUnits, 1_024_000, "KiB"},
		"MiB":              {byteUnits, 3 << 20, "MiB"},
		"half MiB":         {byteUnits, 3 << 19, "MiB"},
		"GB":               {byteUnits, 2e9, "GB"},
		"GiB":              {byteUnits, 5 << 30, "GiB"},
		"TiB":              {byteUnits, 1 << 40, "GiB"},
		"Fast 4G down":     {rateUnits, 1_012_500, "Mbit/s"},
		"Fast 4G up":       {rateUnits, 168_750, "Mbit/s"},
		"Slow 4G up":       {rateUnits, 84_375, "kbit/s"},
		"3G":               {rateUnits, 50_000, "KB/s"},
		"KB/s over kbit":   {rateUnits, 16_000, "KB/s"},
		"Mbit/s":           {rateUnits, 125_000, "Mbit/s"},
		"MB/s ties Mbit/s": {rateUnits, 1_000_000, "MB/s"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, defaultUnit(tc.units, tc.v).name)
		})
	}
}

// TestUnitParse tests reading a number in a unit as bytes or bytes per second,
// rounded to the nearest whole, and the numbers it refuses.
func TestUnitParse(t *testing.T) {
	for name, tc := range map[string]struct {
		units    []valueUnit
		in, unit string
		want     int64
		err      string
	}{
		"B":            {units: byteUnits, in: "1024", unit: "B", want: 1024},
		"KiB fraction": {units: byteUnits, in: "1.5", unit: "KiB", want: 1536},
		"MB fraction":  {units: byteUnits, in: "1.1", unit: "MB", want: 1_100_000},
		"GiB":          {units: byteUnits, in: "2", unit: "GiB", want: 2 << 30},
		"rounds":       {units: byteUnits, in: "0.0005", unit: "KB", want: 1},
		"empty":        {units: byteUnits, in: " ", unit: "MB", want: 0},
		"Mbit/s":       {units: rateUnits, in: "1.5", unit: "Mbit/s", want: 187_500},
		"kbit/s":       {units: rateUnits, in: "128", unit: "kbit/s", want: 16_000},
		"B fraction":   {units: byteUnits, in: "1.5", unit: "B", err: `"1.5" is not a whole number`},
		"B/s fraction": {units: rateUnits, in: "0.5", unit: "B/s", err: `"0.5" is not a whole number`},
		"not a number": {units: byteUnits, in: "abc", unit: "KiB", err: `"abc" is not a number`},
		"NaN":          {units: byteUnits, in: "NaN", unit: "KiB", err: `"NaN" is not a number`},
		"too large":    {units: byteUnits, in: "1e10", unit: "GiB", err: "1e10 GiB is too large"},
	} {
		t.Run(name, func(t *testing.T) {
			u, ok := lookupUnit(tc.units, tc.unit)
			require.True(t, ok)
			got, err := u.parse(tc.in)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestUnitFormat tests showing a value in a unit with no trailing zeros.
func TestUnitFormat(t *testing.T) {
	for name, tc := range map[string]struct {
		units      []valueUnit
		v          int64
		unit, want string
	}{
		"B":        {byteUnits, 1024, "B", "1024"},
		"KiB":      {byteUnits, 1536, "KiB", "1.5"},
		"KB":       {byteUnits, 1536, "KB", "1.536"},
		"whole MB": {byteUnits, 3e6, "MB", "3"},
		"small GB": {byteUnits, 1, "GB", "0.000000001"},
		"Mbit/s":   {rateUnits, 1_012_500, "Mbit/s", "8.1"},
		"kbit/s":   {rateUnits, 84_375, "kbit/s", "675"},
	} {
		t.Run(name, func(t *testing.T) {
			u, ok := lookupUnit(tc.units, tc.unit)
			require.True(t, ok)
			require.Equal(t, tc.want, u.format(tc.v))
		})
	}
}

// TestScope tests the endpoint query parameter. It names an endpoint of the app
// by its handler and any other by its pattern, which a pattern of an endpoint
// of the app also still does.
func TestScope(t *testing.T) {
	eps := []toxic.Endpoint{
		{Pattern: "GET /{$}", Owner: "PageIndex", Handler: "GET"},
		{Pattern: "GET /_$/{$}", Owner: "PageIndex", Handler: "GETStream"},
		{Pattern: "GET /static/", Handler: "assets"},
	}
	for name, tc := range map[string]struct {
		key, pattern, canonical string
	}{
		"global":          {"", "", ""},
		"handler":         {"PageIndex.GETStream", "GET /_$/{$}", "PageIndex.GETStream"},
		"assets":          {"assets", "GET /static/", "assets"},
		"pattern":         {"GET /_$/{$}", "GET /_$/{$}", "PageIndex.GETStream"},
		"unknown pattern": {"POST /gone/{$}", "POST /gone/{$}", "POST /gone/{$}"},
		"unknown handler": {"PageGone.GET", "PageGone.GET", "PageGone.GET"},
	} {
		t.Run(name, func(t *testing.T) {
			pattern := scopePattern(tc.key, eps)
			require.Equal(t, tc.pattern, pattern)
			require.Equal(t, tc.canonical, scopeKey(pattern, eps))
		})
	}
}
