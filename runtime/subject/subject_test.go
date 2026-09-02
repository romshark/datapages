package subject_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/subject"
)

// TestEncode tests which bytes survive a value's trip into a subject.
// What NATS reads as structure or a separator is percent-escaped, the escape character
// itself included; the rest, Unicode included, is left alone. Every case also asserts
// that EncodedLen agrees and that the result is one token.
func TestEncode(t *testing.T) {
	for name, tt := range map[string]struct{ in, want string }{
		"plain":           {"alice", "alice"},
		"digits":          {"u42", "u42"},
		"dash underscore": {"user-42_x", "user-42_x"},
		"at sign":         {"alice@example", "alice@example"},
		"unicode":         {"josé", "josé"},
		"dot":             {"a.b", "a%2Eb"},
		"email":           {"alice@example.com", "alice@example%2Ecom"},
		"star":            {"a*b", "a%2Ab"},
		"gt":              {"a>b", "a%3Eb"},
		"space":           {"a b", "a%20b"},
		"tab":             {"a\tb", "a%09b"},
		"cr":              {"a\rb", "a%0Db"},
		"lf":              {"a\nb", "a%0Ab"},
		"escape itself":   {"100%", "100%25"},
		"wildcard alone":  {"*", "%2A"},
		"gt alone":        {">", "%3E"},
		"empty":           {"", ""},
		"already escaped": {"a%2Eb", "a%252Eb"},
	} {
		t.Run(name, func(t *testing.T) {
			got := subject.Encode(tt.in)
			require.Equal(t, tt.want, got)
			require.Equal(t, len(got), subject.EncodedLen(tt.in),
				"EncodedLen disagrees with Encode")
			if tt.in != "" {
				require.True(t, subject.IsToken(got),
					"the encoded value is not a subject token")
			}
		})
	}
}

// TestEncodeIsInjective tests values that differ only in what gets escaped.
// Two of them must not collide, or one user would read another's events.
func TestEncodeIsInjective(t *testing.T) {
	values := []string{
		"a.b", "a%2Eb", "a%252Eb", "ab", "a b", "a%20b", "a*b", "a%2Ab",
		"100%", "100%25", ">", "%3E",
	}
	seen := make(map[string]string, len(values))
	for _, v := range values {
		got := subject.Encode(v)
		prev, dup := seen[got]
		require.False(t, dup, "%q and %q both encode to %q", prev, v, got)
		seen[got] = v
	}
}

// TestEncodeUnescapedDoesNotAllocate tests the common case:
// an ID that needs no escaping is returned as it is.
func TestEncodeUnescapedDoesNotAllocate(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		_ = subject.Encode("alice@example-42")
	})
	require.Zero(t, n, "Encode allocated for a value that needs no escaping")
}

// FuzzEncode tests the two invariants over arbitrary input: EncodedLen predicts the
// length Encode produces, and a non-empty value always encodes to a single subject token.
func FuzzEncode(f *testing.F) {
	for _, seed := range []string{
		"", "a", "a.b", "alice@example.com", "*", ">", "%", "100%",
		strings.Repeat(".", 8), strings.Repeat("a", 9), "\t\r\n ",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, v string) {
		got := subject.Encode(v)
		require.Equal(t, len(got), subject.EncodedLen(v),
			"EncodedLen disagrees with Encode for %q", v)
		if v != "" {
			require.True(t, subject.IsToken(got),
				"Encode(%q) = %q is not a subject token", v, got)
		}
	})
}
