package sse_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// TestSelectorLineBreak covers the selector that would end the data line it goes on.
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

// TestRemoveElementMatchesDatastar covers the removal event the wrapper writes.
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

// TestRedirectCarriesNoTag covers a redirect target that would end the script element
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
