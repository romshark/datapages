package actionexpr_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/actionexpr"
)

func writeOptions(options []actionexpr.Option) string {
	var b strings.Builder
	actionexpr.WriteOptions(&b, options)
	return b.String()
}

// TestWriteOptions tests the JavaScript options object the action expression carries.
// An option that produces no entry writes nothing at all, not an empty object,
// and a value that could end the JS string or the attribute it sits in is escaped.
func TestWriteOptions(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		options []actionexpr.Option
		want    string
	}{
		"none": {nil, ""},
		"only before and after": {
			[]actionexpr.Option{
				actionexpr.WithBefore("a()"), actionexpr.WithAfter("b()"),
			},
			"",
		},
		"empty headers write nothing": {
			[]actionexpr.Option{actionexpr.WithHeaders(nil)},
			"",
		},
		"one": {
			[]actionexpr.Option{actionexpr.WithRetry(actionexpr.RetryNever)},
			", {retry: 'never'}",
		},
		"two": {
			[]actionexpr.Option{
				actionexpr.WithRetryInterval(500),
				actionexpr.WithContentType(actionexpr.ContentTypeForm),
			},
			", {retryInterval: 500, contentType: 'form'}",
		},
		"selector escapes": {
			[]actionexpr.Option{actionexpr.WithSelector(`#a'b\c`)},
			`, {selector: '#a\'b\\c'}`,
		},
		// A raw line break ends the JS string and the attribute with it.
		"selector escapes line breaks": {
			[]actionexpr.Option{actionexpr.WithSelector("#a\nb\rc")},
			`, {selector: '#a\nb\rc'}`,
		},
		"headers escape line breaks": {
			[]actionexpr.Option{
				actionexpr.WithHeaders(map[string]string{"X-A": "1\n2"}),
			},
			`, {headers: {'X-A': '1\n2'}}`,
		},
		"filter signals": {
			[]actionexpr.Option{actionexpr.WithFilterSignals("foo", "bar")},
			", {filterSignals: {include: /foo/, exclude: /bar/}}",
		},
		"filter signals include all": {
			[]actionexpr.Option{actionexpr.WithFilterSignals("", "")},
			", {filterSignals: {include: /.*/}}",
		},
		"headers": {
			[]actionexpr.Option{
				actionexpr.WithHeaders(map[string]string{"X-A": "1"}),
			},
			", {headers: {'X-A': '1'}}",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, writeOptions(tc.options))
		})
	}
}

// TestLenMatchesWrite pins what the generated helpers rely on: the length
// functions size the strings.Builder the write functions then fill, and a
// wrong size costs a reallocation on every action expression.
func TestLenMatchesWrite(t *testing.T) {
	t.Parallel()

	for name, options := range map[string][]actionexpr.Option{
		"none": nil,
		"every kind": {
			actionexpr.WithBefore("before1()"),
			actionexpr.WithRetry(actionexpr.RetryAlways),
			actionexpr.WithAfter("after1()"),
			actionexpr.WithHeaders(map[string]string{"X-A": "1"}),
			actionexpr.WithBefore("before2()"),
			actionexpr.WithSelector("#form"),
			actionexpr.WithAfter("after2()"),
		},
		"no entry, only before": {
			actionexpr.WithBefore("a()"),
			actionexpr.WithHeaders(nil),
		},
		"option without before or after": {
			actionexpr.WithPayload("$x"),
			actionexpr.WithOpenWhenHidden(true),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var before, after strings.Builder
			actionexpr.WriteBefore(&before, options)
			actionexpr.WriteAfter(&after, options)
			bl, al := actionexpr.BeforeAfterLen(options)

			require.Equal(t, before.Len(), bl, "before length")
			require.Equal(t, after.Len(), al, "after length")
			require.Equal(t,
				len(writeOptions(options)), actionexpr.OptionsLen(options))
		})
	}
}

// TestBeforeAfterOrder tests that before and after snippets run in the order
// they were passed, whichever order the options themselves came in.
func TestBeforeAfterOrder(t *testing.T) {
	t.Parallel()

	options := []actionexpr.Option{
		actionexpr.WithBefore("a()"),
		actionexpr.WithAfter("c()"),
		actionexpr.WithBefore("b()"),
		actionexpr.WithAfter("d()"),
	}

	var before, after strings.Builder
	actionexpr.WriteBefore(&before, options)
	actionexpr.WriteAfter(&after, options)

	require.Equal(t, "a(); b(); ", before.String())
	require.Equal(t, "; c(); d()", after.String())
}

// TestWithOption tests the escape hatch for an option this package does not know.
// An empty key writes nothing, since it would produce invalid JavaScript.
func TestWithOption(t *testing.T) {
	t.Parallel()

	require.Equal(t, ", {custom: $x}",
		writeOptions([]actionexpr.Option{actionexpr.WithOption("custom", "$x")}))
	require.Empty(t,
		writeOptions([]actionexpr.Option{actionexpr.WithOption("", "$x")}))
}
