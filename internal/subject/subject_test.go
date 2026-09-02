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
