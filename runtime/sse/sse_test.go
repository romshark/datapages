package sse_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/sse"
)

// recorder captures what a generator writes.
type recorder struct {
	h   http.Header
	buf strings.Builder
}

func (r *recorder) Header() http.Header         { return r.h }
func (r *recorder) Write(b []byte) (int, error) { return r.buf.Write(b) }
func (r *recorder) WriteHeader(int)             {}
func (r *recorder) Flush()                      {}

// frame runs fn against a fresh generator and returns what it wrote.
func frame(t *testing.T, fn func(g *datastar.ServerSentEventGenerator) error) string {
	t.Helper()
	w := &recorder{h: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/_$/", nil)
	require.NoError(t, fn(datastar.NewSSE(w, req)))
	return w.buf.String()
}

var element = templ.Raw(`<div id="out">x</div>`)

// TestPatchElement tests the element patch event the wrapper writes. An empty selector,
// an empty mode and a mode Datastar does not know are all left out of the event
// rather than written empty, which would make the client patch the wrong place.
func TestPatchElement(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		call func(s datapages.SSE) error
		want []string
		omit []string
	}{
		"plain": {
			call: func(s datapages.SSE) error { return s.PatchElement(element) },
			want: []string{"event: datastar-patch-elements", `data: elements <div id="out">x</div>`},
			omit: []string{"data: selector", "data: mode"},
		},
		"selector and mode": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "#t", datapages.PatchModeAppend)
			},
			want: []string{"data: selector #t", "data: mode append"},
		},
		"selector only": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "#t", "")
			},
			want: []string{"data: selector #t"},
			omit: []string{"data: mode"},
		},
		"mode only": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "", datapages.PatchModeInner)
			},
			want: []string{"data: mode inner"},
			omit: []string{"data: selector"},
		},
		"neither": {
			call: func(s datapages.SSE) error { return s.PatchElementAt(element, "", "") },
			omit: []string{"data: selector", "data: mode"},
		},
		"unknown mode is ignored": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "", datapages.PatchMode("sideways"))
			},
			omit: []string{"data: mode", "data: selector"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
				return tc.call(sse.New(g))
			})
			for _, want := range tc.want {
				require.Contains(t, got, want)
			}
			for _, omit := range tc.omit {
				require.NotContains(t, got, omit)
			}
		})
	}
}

// TestSelectorLineBreak tests the selector that would end the data line it goes on.
// What follows one reaches the browser as events of its own.
func TestSelectorLineBreak(t *testing.T) {
	t.Parallel()

	const injection = "#a\n\nevent: datastar-execute-script\ndata: script alert(1)"
	for name, tc := range map[string]struct {
		call func(s datapages.SSE) error
	}{
		"remove newline": {
			call: func(s datapages.SSE) error { return s.RemoveElement(injection) },
		},
		"remove carriage return": {
			call: func(s datapages.SSE) error { return s.RemoveElement("#a\rfoo") },
		},
		"patch at newline": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, injection, datapages.PatchModeInner)
			},
		},
		"patch at carriage return": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "#a\rfoo", "")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := &recorder{h: make(http.Header)}
			req := httptest.NewRequest(http.MethodGet, "/_$/", nil)
			err := tc.call(sse.New(datastar.NewSSE(w, req)))
			require.ErrorIs(t, err, datapages.ErrSelectorLineBreak)
			require.Empty(t, w.buf.String(), "the event reached the wire")
		})
	}
}

// TestRemoveElementMatchesDatastar tests the removal event the wrapper writes.
// Its bytes must be the ones datastar.RemoveElement writes.
func TestRemoveElementMatchesDatastar(t *testing.T) {
	t.Parallel()

	want := frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return g.RemoveElement("#gone")
	})
	got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).RemoveElement("#gone")
	})
	require.Equal(t, want, got)
}

// TestRedirectCarriesNoTag tests a redirect target that would end the script element
// it travels in. What follows such a target would reach the DOM as markup of its own.
func TestRedirectCarriesNoTag(t *testing.T) {
	t.Parallel()

	got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).Redirect(`/x</script><img src=x onerror=alert(1)>`)
	})

	require.NotContains(t, got, "</script><img",
		"the target ended the script element:\n%s", got)
	require.Contains(t, got, `\u003c/script\u003e`,
		"the target is not encoded:\n%s", got)
	require.Contains(t, got, "window.location.href")
}

// TestPatchSignals tests the signal patch event for a struct, for raw JSON and
// for the if-missing variant. A value that fails to marshal returns the error and
// writes nothing: a half-written event would leave the stream unparseable.
func TestPatchSignals(t *testing.T) {
	t.Parallel()

	type signals struct {
		Count int `json:"count"`
	}

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
			return sse.New(g).PatchSignals(signals{Count: 7})
		})
		require.Contains(t, got, "event: datastar-patch-signals")
		require.Contains(t, got, `data: signals {"count":7}`)
		require.NotContains(t, got, "onlyIfMissing")
	})

	t.Run("raw json", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
			return sse.New(g).PatchSignals(json.RawMessage(`{"count":7}`))
		})
		require.Contains(t, got, `data: signals {"count":7}`)
	})

	t.Run("if missing", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
			return sse.New(g).PatchSignalsIfMissing(signals{Count: 3})
		})
		require.Contains(t, got, "data: onlyIfMissing true")
		require.Contains(t, got, `data: signals {"count":3}`)
	})

	t.Run("invalid raw json", func(t *testing.T) {
		t.Parallel()
		w := &recorder{h: make(http.Header)}
		req := httptest.NewRequest(http.MethodGet, "/_$/", nil)
		err := sse.New(datastar.NewSSE(w, req)).
			PatchSignals(json.RawMessage(`{"count":`))
		require.Error(t, err)
		require.NotContains(t, w.buf.String(), "datastar-patch-signals")
	})

	t.Run("value that does not marshal", func(t *testing.T) {
		t.Parallel()
		w := &recorder{h: make(http.Header)}
		req := httptest.NewRequest(http.MethodGet, "/_$/", nil)
		err := sse.New(datastar.NewSSE(w, req)).PatchSignals(make(chan int))
		require.Error(t, err)
		require.NotContains(t, w.buf.String(), "datastar-patch-signals")
	})
}

// TestScriptAndRedirect tests the three events that carry a payload straight to
// the browser: a script, a redirect target and prefetch hints.
func TestScriptAndRedirect(t *testing.T) {
	t.Parallel()

	got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).ExecuteScript(`console.log("x")`)
	})
	require.Contains(t, got, `console.log("x")`)

	got = frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).Redirect("/next/")
	})
	require.Contains(t, got, "/next/")

	got = frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).Prefetch("/a/", "/b/")
	})
	require.Contains(t, got, "/a/")
	require.Contains(t, got, "/b/")
}

// prefetchURLs extracts URLs after validating the SSE, script and JSON boundaries.
func prefetchURLs(t *testing.T, event string) []string {
	t.Helper()
	require.NotContains(t, event, "\r",
		"a carriage return reached the event stream:\n%s", event)
	var lines []string
	for line := range strings.Lines(event) {
		if el, ok := strings.CutPrefix(line, "data: elements "); ok {
			lines = append(lines, strings.TrimSuffix(el, "\n"))
		}
	}
	element := strings.Join(lines, "\n")
	body, ok := strings.CutPrefix(element, `<script type="speculationrules">`)
	require.True(t, ok, "not a speculation rules element:\n%s", element)
	body, ok = strings.CutSuffix(body, "</script>")
	require.True(t, ok, "the element does not end the script:\n%s", element)
	// Escaping every '<' prevents </script> and <!-- from changing the element.
	require.NotContains(t, body, "<",
		"a URL put a '<' into the script element:\n%s", element)

	var rules struct {
		Prefetch []struct {
			Source string   `json:"source"`
			URLs   []string `json:"urls"`
		} `json:"prefetch"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &rules),
		"the rules are not JSON:\n%s", body)
	require.Len(t, rules.Prefetch, 1)
	require.Equal(t, "list", rules.Prefetch[0].Source)
	return rules.Prefetch[0].URLs
}

// TestPrefetchEncodesURLs tests that control and markup bytes cross the SSE,
// script and JSON layers without changing a URL.
func TestPrefetchEncodesURLs(t *testing.T) {
	t.Parallel()

	for name, url := range map[string]string{
		"quote":           `/u/x"y`,
		"backslash":       `/u/x\y`,
		"end of script":   `/u/x"</script><b>hi</b>`,
		"comment":         "/u/<!--<script>",
		"line feed":       "/u/x\ny",
		"carriage return": "/u/x\ry",
		"query":           "/search/?q=a&page=2",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
				return sse.New(g).Prefetch("/a/", url)
			})
			require.Equal(t, []string{"/a/", url}, prefetchURLs(t, got))
		})
	}
}

func TestPrefetchWithoutURLs(t *testing.T) {
	t.Parallel()

	got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
		return sse.New(g).Prefetch()
	})
	require.Empty(t, got)
}

// FuzzPrefetch tests that arbitrary URLs produce safe rules. Valid UTF-8 URLs
// round-trip unchanged because encoding/json replaces invalid UTF-8 with U+FFFD.
func FuzzPrefetch(f *testing.F) {
	for _, seed := range [][2]string{
		{"/a/", "/b/"},
		{`/u/x"</script><b>hi</b>`, `/a\b`},
		{"/u/<!--<script>", "/x\r\ny"},
		{"", "\x00\x1f\x7f"},
		{"/\u00e4/", "\xff\xfe"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		got := frame(t, func(g *datastar.ServerSentEventGenerator) error {
			return sse.New(g).Prefetch(a, b)
		})
		urls := prefetchURLs(t, got)
		if utf8.ValidString(a) && utf8.ValidString(b) {
			require.Equal(t, []string{a, b}, urls)
		}
	})
}

type discard struct{ h http.Header }

func (d *discard) Header() http.Header         { return d.h }
func (d *discard) Write(b []byte) (int, error) { return len(b), nil }
func (d *discard) WriteHeader(int)             {}
func (d *discard) Flush()                      {}

// BenchmarkPrefetch measures the href-generated URL case.
func BenchmarkPrefetch(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/_$/", nil)
	s := sse.New(datastar.NewSSE(&discard{h: make(http.Header)}, req))
	urls := []string{"/listing/42/", "/listing/43/", "/search/?q=bike&page=2"}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Prefetch(urls...); err != nil {
			b.Fatal(err)
		}
	}
}
