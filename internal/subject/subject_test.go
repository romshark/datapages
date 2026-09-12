package subject_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/subject"
)

// TestIsToken tests what may stand as one subject token: anything without a separator,
// a wildcard or whitespace, Unicode included. An empty value is no token either.
func TestIsToken(t *testing.T) {
	t.Parallel()
	for name, td := range map[string]struct {
		value string
		want  bool
	}{
		"plain":          {value: "user42", want: true},
		"dashes":         {value: "a-b_c", want: true},
		"unicode":        {value: "müller", want: true},
		"empty":          {value: ""},
		"separator":      {value: "a.b"},
		"star":           {value: "a*"},
		"full wildcard":  {value: ">"},
		"space":          {value: "a b"},
		"tab":            {value: "a\tb"},
		"newline":        {value: "a\nb"},
		"carriage retrn": {value: "a\rb"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.want, subject.IsToken(td.value))
		})
	}
}

// TestPrefix tests the separator the prefix ends in,
// which is what keeps "a.b" from matching "a.bc".
func TestPrefix(t *testing.T) {
	t.Parallel()
	require.Equal(t, "messaging.sent.", subject.Prefix("messaging.sent"))
}

// TestClaimOverlaps tests which two event declarations claim the same subjects.
// A claim with subject fields stands for every subject under its prefix,
// which makes it overlap anything nested below it, while two plain claims overlap only
// when equal. A shared textual prefix without a separator between them is no overlap.
// Every case is asserted both ways round, since the relation is symmetric.
func TestClaimOverlaps(t *testing.T) {
	t.Parallel()
	fields := func(s string) subject.Claim {
		return subject.Claim{Subject: s, HasFields: true}
	}
	plain := func(s string) subject.Claim { return subject.Claim{Subject: s} }

	for name, td := range map[string]struct {
		a, b subject.Claim
		want bool
	}{
		"plain equal":            {a: plain("a.b"), b: plain("a.b")},
		"plain different":        {a: plain("a.b"), b: plain("a.c")},
		"fields over plain":      {a: fields("a"), b: plain("a.b"), want: true},
		"plain under fields":     {a: plain("a.b"), b: fields("a"), want: true},
		"plain beside fields":    {a: plain("ab"), b: fields("a")},
		"plain equals fields":    {a: plain("a"), b: fields("a")},
		"fields nested":          {a: fields("a"), b: fields("a.b"), want: true},
		"fields nested reversed": {a: fields("a.b"), b: fields("a"), want: true},
		"fields equal":           {a: fields("a.b"), b: fields("a.b"), want: true},
		"fields siblings":        {a: fields("a.b"), b: fields("a.c")},
		"fields shared prefix":   {a: fields("ab"), b: fields("a")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, td.want, td.a.Overlaps(td.b))
			require.Equal(t, td.want, td.b.Overlaps(td.a))
		})
	}
}

// TestCheckAcrossApps tests the subject rule between the applications of one module.
// Two of them claiming one subject deliver each other's events,
// while two events of one application are left to the check the parser runs.
func TestCheckAcrossApps(t *testing.T) {
	t.Parallel()
	// Every application declares its event itself unless a case says otherwise.
	ev := func(app, typeName, subj string) subject.AppEvent {
		return subject.AppEvent{
			App: app, TypeName: typeName, Decl: app + "." + typeName,
			Claim: subject.Claim{Subject: subj},
		}
	}
	// shared is one declaration two applications take part in.
	shared := func(app, typeName, subj string) subject.AppEvent {
		e := ev(app, typeName, subj)
		e.Decl = "events." + typeName
		return e
	}
	evFields := func(app, typeName, subj string) subject.AppEvent {
		e := ev(app, typeName, subj)
		e.HasFields = true
		return e
	}

	for name, td := range map[string]struct {
		events  []subject.AppEvent
		wantErr bool
	}{
		"equal subjects in two apps": {
			events: []subject.AppEvent{
				ev("app/alpha", "EventPing", "ping"),
				ev("app/beta", "EventPing", "ping"),
			},
			wantErr: true,
		},
		"equal subjects in one app": {
			// The parser refuses this one; here it must stay silent.
			events: []subject.AppEvent{
				ev("app/alpha", "EventPing", "ping"),
				ev("app/alpha", "EventPong", "ping"),
			},
		},
		"nested under a claim with fields": {
			events: []subject.AppEvent{
				evFields("app/alpha", "EventNotify", "notify"),
				ev("app/beta", "EventNotifyUser", "notify.user"),
			},
			wantErr: true,
		},
		"different subjects": {
			events: []subject.AppEvent{
				ev("app/alpha", "EventPing", "ping"),
				ev("app/beta", "EventPong", "pong"),
			},
		},
		"shared text without a separator": {
			events: []subject.AppEvent{
				evFields("app/alpha", "EventPing", "ping"),
				ev("app/beta", "EventPinger", "pinger"),
			},
		},
		"one declaration in two apps": {
			events: []subject.AppEvent{
				shared("app/alpha", "EventPing", "ping"),
				shared("app/beta", "EventPing", "ping"),
			},
		},
		"no events": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := subject.CheckAcrossApps(td.events)
			if !td.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, e := range td.events {
				require.Contains(t, err.Error(), e.App)
				require.Contains(t, err.Error(), e.TypeName)
			}
		})
	}
}
