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

// TestSetSlot tests the modes the settings page sets a slot to.
// A toxic turned off or back to inherit keeps the values it had,
// and an endpoint without overrides leaves no empty entry behind.
func TestSetSlot(t *testing.T) {
	const ep = "GET /{$}"
	disabled := func(x toxic.Toxic) toxic.Toxic { x.Disabled = true; return x }
	// globalOnly returns settings with the global toxics g and no endpoints.
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

// TestMatches tests the fuzzy search of the endpoint list: each term occurs
// in order, ignoring case, and the terms independently of each other.
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

// TestPathParts tests the pieces of a route path the page colors:
// wildcards, a wildcard that matches the rest, and the end anchor.
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

// TestFilter tests the kinds of the endpoint list: empty is every kind,
// "none", which the page sends with no toggle pressed, is no kind.
func TestFilter(t *testing.T) {
	for _, tc := range []struct {
		kinds   string
		pressed string
		all     bool
	}{
		{kinds: "", pressed: "get,stream,action,file", all: true},
		{kinds: "file,stream,get,action", pressed: "get,stream,action,file", all: true},
		{kinds: "action,get", pressed: "get,action", all: false},
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

// TestSummary tests the line an endpoint shows for the global toxic it
// inherits, which leaves out the unit of a field without one.
func TestSummary(t *testing.T) {
	require.Equal(t, "Latency 300 ms, Jitter 0 ms, 100% of requests",
		summary(toxic.Types[toxic.Latency], toxicWith(300, 0)))
	half := toxicWith(503)
	half.Toxicity = 0.5
	require.Equal(t, "Status 503, 50% of requests",
		summary(toxic.Types[toxic.HTTPError], half))
}

// TestStatusOptions tests the statuses the combobox offers: those net/http
// names from 400 to 599, and a status datapages.yaml sets that it does not name.
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

// TestSetField tests setting 1 field of a toxic from the text of the page:
// a field by its key, the percentage of requests affected, an empty field as 0,
// and the errors shown in the row of the toxic.
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
