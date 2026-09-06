package actionexpr_test

import (
	"bytes"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/actionexpr"
)

func writeOptions(options []actionexpr.Option) string {
	var b strings.Builder
	actionexpr.WriteOptions(&b, options)
	return b.String()
}

// TestInvalidOptionsAreDroppedAndLogged tests every option that can be handed
// a value the expression cannot carry. Writing it would throw in the browser
// and take the whole action call with it, so the option is left out and the
// action runs on the Datastar default. Silence would leave nothing to look at.
func TestInvalidOptionsAreDroppedAndLogged(t *testing.T) {
	// The option is built inside the subtest: a value built with the table
	// would log before the subtest installs its own logger.
	for name, tc := range map[string]struct {
		option func() actionexpr.Option
		want   string // rendered expression, "" when the option is dropped
		logged string // substring of the warning, "" when nothing is logged
	}{
		"retry scaler positive infinity": {
			option: func() actionexpr.Option { return actionexpr.WithRetryScaler(math.Inf(1)) },
			logged: "value=+Inf",
		},
		"retry scaler negative infinity": {
			option: func() actionexpr.Option { return actionexpr.WithRetryScaler(math.Inf(-1)) },
			logged: "value=-Inf",
		},
		"retry scaler not a number": {
			option: func() actionexpr.Option { return actionexpr.WithRetryScaler(math.NaN()) },
			logged: "value=NaN",
		},
		"retry scaler finite": {
			option: func() actionexpr.Option { return actionexpr.WithRetryScaler(1.5) },
			want:   ", {retryScaler: 1.5}",
		},
		"retry unknown": {
			option: func() actionexpr.Option { return actionexpr.WithRetry(actionexpr.Retry("bogus")) },
			logged: "WithRetry",
		},
		"retry known": {
			option: func() actionexpr.Option { return actionexpr.WithRetry(actionexpr.RetryNever) },
			want:   ", {retry: 'never'}",
		},
		"content type unknown": {
			option: func() actionexpr.Option { return actionexpr.WithContentType(actionexpr.ContentType("xml")) },
			logged: "WithContentType",
		},
		"content type known": {
			option: func() actionexpr.Option { return actionexpr.WithContentType(actionexpr.ContentTypeForm) },
			want:   ", {contentType: 'form'}",
		},
		"request cancellation unknown": {
			option: func() actionexpr.Option {
				return actionexpr.WithRequestCancellation(
					actionexpr.RequestCancellation("later"),
				)
			},
			logged: "WithRequestCancellation",
		},
		"request cancellation known": {
			option: func() actionexpr.Option {
				return actionexpr.WithRequestCancellation(
					actionexpr.RequestCancellationDisabled,
				)
			},
			want: ", {requestCancellation: 'disabled'}",
		},
		"retry interval negative": {
			option: func() actionexpr.Option { return actionexpr.WithRetryInterval(-5) },
			logged: "value=-5",
		},
		"retry interval zero": {
			option: func() actionexpr.Option { return actionexpr.WithRetryInterval(0) },
			want:   ", {retryInterval: 0}",
		},
		"retry max wait negative": {
			option: func() actionexpr.Option { return actionexpr.WithRetryMaxWaitMs(-1) },
			logged: "WithRetryMaxWaitMs",
		},
		"retry max count negative": {
			option: func() actionexpr.Option { return actionexpr.WithRetryMaxCount(-1) },
			logged: "WithRetryMaxCount",
		},
		"retry max count zero": {
			option: func() actionexpr.Option { return actionexpr.WithRetryMaxCount(0) },
			want:   ", {retryMaxCount: 0}",
		},
		// A line terminator ends the regex literal the pattern goes into,
		// and no escape puts it back.
		"filter signals line terminator": {
			option: func() actionexpr.Option { return actionexpr.WithFilterSignals("a\nb", "") },
			logged: "WithFilterSignals",
		},
		// A "/" ends the literal too, but escaping keeps the pattern.
		"filter signals slash": {
			option: func() actionexpr.Option { return actionexpr.WithFilterSignals("a/b", "") },
			want:   `, {filterSignals: {include: /a\/b/}}`,
		},
		"filter signals escaped slash stays": {
			option: func() actionexpr.Option { return actionexpr.WithFilterSignals(`a\/b`, "") },
			want:   `, {filterSignals: {include: /a\/b/}}`,
		},
		"filter signals plain": {
			option: func() actionexpr.Option { return actionexpr.WithFilterSignals("^x", "^_") },
			want:   ", {filterSignals: {include: /^x/, exclude: /^_/}}",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			actionexpr.SetLogger(slog.New(slog.NewTextHandler(&buf, nil)))
			t.Cleanup(func() { actionexpr.SetLogger(nil) })

			require.Equal(t, tc.want,
				writeOptions([]actionexpr.Option{tc.option()}))
			if tc.logged == "" {
				require.Empty(t, buf.String(), "a valid value was logged")
				return
			}
			require.Contains(t, buf.String(), tc.logged)
		})
	}
}

// countingMetrics records what the expression writer refuses.
type countingMetrics struct {
	lock    sync.Mutex
	dropped []string
}

func (c *countingMetrics) OptionDropped(option string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.dropped = append(c.dropped, option)
}

// TestDroppedOptionIsCounted tests the counter behind a dropped option.
// The log is throttled, so the count is what says how often it still happens.
func TestDroppedOptionIsCounted(t *testing.T) {
	var m countingMetrics
	actionexpr.SetMetrics(&m)
	t.Cleanup(func() { actionexpr.SetMetrics(nil) })

	for range 3 {
		actionexpr.WithRetryScaler(math.Inf(1))
	}
	actionexpr.WithRetryInterval(-1)
	actionexpr.WithRetryInterval(1000)

	require.Equal(t, []string{
		"WithRetryScaler", "WithRetryScaler", "WithRetryScaler",
		"WithRetryInterval",
	}, m.dropped)
}

// TestWithHeadersIsStable tests that one call renders one string.
// Go randomizes map iteration, and a single render matches
// sorted order often enough to pass by chance.
func TestWithHeadersIsStable(t *testing.T) {
	t.Parallel()

	headers := map[string]string{"X-C": "3", "X-A": "1", "X-B": "2"}
	want := writeOptions([]actionexpr.Option{actionexpr.WithHeaders(headers)})
	for range 100 {
		require.Equal(t, want,
			writeOptions([]actionexpr.Option{actionexpr.WithHeaders(headers)}))
	}
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
		// "+Inf" is no JavaScript number literal and Inf no identifier, so the
		// expression would throw and send no request at all.
		"positive infinite retry scaler": {
			[]actionexpr.Option{actionexpr.WithRetryScaler(math.Inf(1))},
			"",
		},
		"negative infinite retry scaler": {
			[]actionexpr.Option{actionexpr.WithRetryScaler(math.Inf(-1))},
			"",
		},
		"not a number retry scaler": {
			[]actionexpr.Option{actionexpr.WithRetryScaler(math.NaN())},
			"",
		},
		"finite retry scaler": {
			[]actionexpr.Option{actionexpr.WithRetryScaler(1.5)},
			", {retryScaler: 1.5}",
		},
		"headers are ordered": {
			[]actionexpr.Option{actionexpr.WithHeaders(map[string]string{
				"X-C": "3", "X-A": "1", "X-B": "2",
			})},
			", {headers: {'X-A': '1', 'X-B': '2', 'X-C': '3'}}",
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
