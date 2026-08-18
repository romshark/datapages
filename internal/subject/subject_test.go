package subject_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/subject"
)

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

func TestPrefix(t *testing.T) {
	t.Parallel()
	require.Equal(t, "messaging.sent.", subject.Prefix("messaging.sent"))
}

func TestGenIsToken(t *testing.T) {
	t.Parallel()
	src := subject.GenIsToken()
	require.True(t, strings.HasPrefix(src, "func isSubjectToken(v string) bool {"))
	require.Contains(t, src, strconv.Quote(subject.Reserved))
}

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
