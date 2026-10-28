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
// an empty field as 0, and the errors.
func TestSetField(t *testing.T) {
	x := toxicWith(100, 50)
	require.NoError(t, setField(&x, toxic.Latency, "jitter", " 20 "))
	require.NoError(t, setField(&x, toxic.Latency, "toxicity", "30"))
	require.NoError(t, setField(&x, toxic.Latency, "latency", ""))
	want := toxicWith(0, 20)
	want.Toxicity = 0.3
	require.Equal(t, want, x)

	require.EqualError(t, setField(&x, toxic.Latency, "jitter", "1.5"),
		`jitter: "1.5" is not a whole number`)
	require.EqualError(t, setField(&x, toxic.Latency, "toxicity", "101"),
		"requests affected must be a whole percentage from 0 to 100")
	require.ErrorIs(t, setField(&x, toxic.Latency, "rate", "1"), errUnknownField)
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
