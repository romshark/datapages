package htmlattr_test

import (
	"html"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/htmlattr"
)

// attrSafe reports whether s can sit inside a double quoted HTML attribute
// without ending it.
func attrSafe(t *testing.T, s string) {
	t.Helper()
	require.NotContains(t, s, `"`, "ends the attribute")
	require.NotContains(t, s, "<", "opens a tag")
	require.NotContains(t, s, ">", "closes a tag")
}

// jsStringSafe reports whether s can sit inside a double quoted HTML attribute
// without ending it, and inside a single quoted JavaScript string
// after the browser decoded the attribute.
func jsStringSafe(t *testing.T, s string) {
	t.Helper()
	attrSafe(t, s)

	// The attribute reaches Datastar decoded, where the value sits in
	// a single quoted string. Every quote of it must be escaped.
	decoded := html.UnescapeString(s)
	for i := range len(decoded) {
		if decoded[i] != '\'' {
			continue
		}
		backslashes := 0
		for j := i - 1; j >= 0 && decoded[j] == '\\'; j-- {
			backslashes++
		}
		require.Equal(t, 1, backslashes%2,
			"quote %d of %q ends the JavaScript string", i, decoded)
	}
}

// TestWritePathValue tests a path segment written into a Datastar attribute.
// It must neither end the attribute nor the JavaScript string inside it, and it has to
// survive the round trip a browser makes of it: HTML unescape, then path unescape,
// back to the value it started as.
func TestWritePathValue(t *testing.T) {
	t.Parallel()

	for name, v := range map[string]string{
		"plain":      "alice",
		"apostrophe": "o'brien",
		"quote":      `a"b`,
		"tag":        "<script>",
		"ampersand":  "a&b",
		"slash":      "a/b",
		"space":      "a b",
		"unicode":    "ä✓",
		"percent":    "100%",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			htmlattr.WritePathValue(&b, v)
			jsStringSafe(t, b.String())

			// The value survives the round trip a browser makes of it.
			unescaped, err := url.PathUnescape(html.UnescapeString(b.String()))
			require.NoError(t, err)
			require.Equal(t, v, unescaped)
		})
	}
}

// TestWriteSignalString tests a string written into a Datastar signal literal.
// Unlike a path value it is not round-tripped, only checked for anything that
// could end the attribute or the JavaScript string it sits in.
func TestWriteSignalString(t *testing.T) {
	t.Parallel()

	for name, v := range map[string]string{
		"plain":      "shoes",
		"apostrophe": "o'brien",
		"backslash":  `a\b`,
		"quote":      `a"b`,
		"tag":        "<script>",
		"newline":    "a\nb",
		"carriage":   "a\rb",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			htmlattr.WriteSignalString(&b, v)
			jsStringSafe(t, b.String())
		})
	}
}

// TestWriteSignalValue tests a number or a bool written unquoted into
// a data-signals attribute. It must not end the attribute, and the browser
// has to decode it back to the JavaScript literal it started as.
func TestWriteSignalValue(t *testing.T) {
	t.Parallel()

	for name, v := range map[string]string{
		"int":    "-42",
		"float":  "-0.25",
		"true":   "true",
		"false":  "false",
		"quote":  `a"b`,
		"tag":    "<script>",
		"entity": "&amp;",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			htmlattr.WriteSignalValue(&b, v)
			attrSafe(t, b.String())
			require.Equal(t, v, html.UnescapeString(b.String()))
		})
	}
}

// FuzzWritePathValue tests the attribute safety and the round trip over arbitrary input.
func FuzzWritePathValue(f *testing.F) {
	for _, seed := range []string{"alice", "o'brien", `a"b`, "<script>", "a&b", "ä"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, v string) {
		var b strings.Builder
		htmlattr.WritePathValue(&b, v)
		jsStringSafe(t, b.String())

		unescaped, err := url.PathUnescape(html.UnescapeString(b.String()))
		require.NoError(t, err)
		require.Equal(t, v, unescaped)
	})
}

// FuzzWriteSignalString tests the attribute safety over arbitrary input.
func FuzzWriteSignalString(f *testing.F) {
	for _, seed := range []string{"shoes", "o'brien", `a\b`, "a\nb", "<script>"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, v string) {
		var b strings.Builder
		htmlattr.WriteSignalString(&b, v)
		jsStringSafe(t, b.String())
	})
}

// FuzzWriteSignalValue tests the attribute safety and the round trip over arbitrary input.
func FuzzWriteSignalValue(f *testing.F) {
	for _, seed := range []string{"-42", "-0.25", "true", `a"b`, "<script>", "&amp;"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, v string) {
		var b strings.Builder
		htmlattr.WriteSignalValue(&b, v)
		attrSafe(t, b.String())
		require.Equal(t, v, html.UnescapeString(b.String()))
	})
}
