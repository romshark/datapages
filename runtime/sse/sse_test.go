package sse_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/sse"
)

type recorder struct {
	h       http.Header
	buf     bytes.Buffer
	flushes int
	err     error // Returned by every Write when set.
}

func (r *recorder) Header() http.Header { return r.h }
func (r *recorder) Write(b []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.buf.Write(b)
}
func (r *recorder) WriteHeader(int) {}
func (r *recorder) Flush()          { r.flushes++ }

// compression is the default compression of a server.
var compression = sse.NewCompression(nil)

func streamRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/_$/", nil)
}

func codingRequest(coding string) *http.Request {
	r := streamRequest()
	r.Header.Set("Accept-Encoding", coding)
	return r
}

var codings = []string{"zstd", "br", "gzip"}

func decoder(t *testing.T, coding string, b []byte) io.Reader {
	t.Helper()
	switch coding {
	case "zstd":
		d, err := zstd.NewReader(bytes.NewReader(b))
		require.NoError(t, err)
		t.Cleanup(d.Close)
		return d
	case "br":
		return brotli.NewReader(bytes.NewReader(b))
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(b))
		require.NoError(t, err)
		return zr
	}
	t.Fatalf("unknown coding %q", coding)
	return nil
}

func decode(t *testing.T, coding string, b []byte) string {
	t.Helper()
	out, err := io.ReadAll(decoder(t, coding, b))
	require.NoError(t, err, "the %s stream is incomplete", coding)
	return string(out)
}

func frame(t testing.TB, fn func(s *sse.Stream) error) string {
	t.Helper()
	w := &recorder{h: make(http.Header)}
	s := sse.Open(w, streamRequest(), compression)
	require.NoError(t, fn(s))
	s.Close()
	require.NoError(t, s.Err())
	return w.buf.String()
}

var element = templ.Raw(`<div id="out">x</div>`)

// TestOpen tests response headers, compression negotiation and the initial flush.
func TestOpen(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		acceptEncoding string
		protoMajor     int
		wantEncoding   string
		wantConnection string
	}{
		"http/1.1": {
			protoMajor: 1, wantConnection: "keep-alive",
		},
		"http/2": {protoMajor: 2},
		"browser": {
			acceptEncoding: "gzip, deflate, br, zstd",
			protoMajor:     2,
			wantEncoding:   "zstd",
		},
		"without zstd": {
			acceptEncoding: "gzip, deflate, br",
			protoMajor:     2,
			wantEncoding:   "br",
		},
		"gzip only": {
			acceptEncoding: "gzip",
			protoMajor:     2,
			wantEncoding:   "gzip",
		},
		"zstd refused": {
			acceptEncoding: "zstd;q=0, gzip",
			protoMajor:     2,
			wantEncoding:   "gzip",
		},
		"any coding": {
			acceptEncoding: "*",
			protoMajor:     2,
			wantEncoding:   "gzip",
		},
		"identity listed": {
			acceptEncoding: "identity",
			protoMajor:     2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := streamRequest()
			r.ProtoMajor = tc.protoMajor
			if tc.acceptEncoding != "" {
				r.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}
			w := &recorder{h: make(http.Header)}
			s := sse.Open(w, r, compression)
			require.NoError(t, s.Err())
			require.Equal(t, 1, w.flushes, "the head is not flushed")
			require.Equal(t, "text/event-stream", w.h.Get("Content-Type"))
			require.Equal(t, "no-cache", w.h.Get("Cache-Control"))
			require.Equal(t, tc.wantConnection, w.h.Get("Connection"))
			require.Equal(t, tc.wantEncoding, w.h.Get("Content-Encoding"))
			require.Equal(t, []string{"Accept-Encoding"}, w.h.Values("Vary"))
			require.Zero(t, w.buf.Len(), "the head wrote body bytes")
		})
	}
}

// TestOpenWithoutFlushPanics tests a writer that hides [http.Flusher].
func TestOpenWithoutFlushPanics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		sse.Open(struct {
			http.ResponseWriter
		}{
			httptest.NewRecorder(),
		}, streamRequest(), compression)
	})
}

// TestPatchElement tests fields, defaults and line framing of an element patch.
func TestPatchElement(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		call func(s datapages.SSE) error
		want string
	}{
		"plain": {
			call: func(s datapages.SSE) error { return s.PatchElement(element) },
			want: "event: datastar-patch-elements\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"selector and mode": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "#t", datapages.PatchModeAppend)
			},
			want: "event: datastar-patch-elements\n" +
				"data: selector #t\n" +
				"data: mode append\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"selector only": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "#t", "")
			},
			want: "event: datastar-patch-elements\n" +
				"data: selector #t\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"mode only": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "", datapages.PatchModeInner)
			},
			want: "event: datastar-patch-elements\n" +
				"data: mode inner\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"outer mode is the default": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "", datapages.PatchModeOuter)
			},
			want: "event: datastar-patch-elements\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"unknown mode is ignored": {
			call: func(s datapages.SSE) error {
				return s.PatchElementAt(element, "", datapages.PatchMode("sideways"))
			},
			want: "event: datastar-patch-elements\n" +
				`data: elements <div id="out">x</div>` + "\n\n",
		},
		"lines": {
			call: func(s datapages.SSE) error {
				return s.PatchElement(templ.Raw("<ul id=\"l\">\n<li>a</li>\n</ul>"))
			},
			want: "event: datastar-patch-elements\n" +
				"data: elements <ul id=\"l\">\n" +
				"data: elements <li>a</li>\n" +
				"data: elements </ul>\n\n",
		},
		"empty": {
			call: func(s datapages.SSE) error { return s.PatchElement(templ.Raw("")) },
			want: "event: datastar-patch-elements\n\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := frame(t, func(s *sse.Stream) error { return tc.call(s) })
			require.Equal(t, tc.want, got)
		})
	}
}

// TestPatchElementRenderError tests that a render failure writes nothing and
// leaves the stream usable.
func TestPatchElementRenderError(t *testing.T) {
	t.Parallel()

	errRender := errors.New("render failed")
	got := frame(t, func(s *sse.Stream) error {
		err := s.PatchElement(templ.ComponentFunc(
			func(_ context.Context, w io.Writer) error {
				_, _ = io.WriteString(w, `<div id="half">`)
				return errRender
			},
		))
		require.ErrorIs(t, err, errRender)
		return s.PatchElement(element)
	})
	require.Equal(t, "event: datastar-patch-elements\n"+
		`data: elements <div id="out">x</div>`+"\n\n", got)
}

// TestSelectorLineBreak tests that CR and LF cannot inject SSE fields.
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
			got := frame(t, func(s *sse.Stream) error {
				require.ErrorIs(t, tc.call(s), datapages.ErrSelectorLineBreak)
				return nil
			})
			require.Empty(t, got, "the event reached the wire")
		})
	}
}

// TestRemoveElement tests a removal event without an elements field.
func TestRemoveElement(t *testing.T) {
	t.Parallel()

	got := frame(t, func(s *sse.Stream) error { return s.RemoveElement("#gone") })
	require.Equal(t, "event: datastar-patch-elements\n"+
		"data: selector #gone\n"+
		"data: mode remove\n\n", got)
}

// patchedElements extracts HTML and rejects invalid element-patch framing.
func patchedElements(t *testing.T, event string) string {
	t.Helper()
	require.NotContains(t, event, "\r",
		"a carriage return reached the event stream:\n%q", event)
	body, ok := strings.CutPrefix(event, "event: datastar-patch-elements\n")
	require.True(t, ok, "not an elements patch:\n%q", event)
	body, ok = strings.CutSuffix(body, "\n")
	require.True(t, ok && (body == "" || strings.HasSuffix(body, "\n")),
		"the event does not end:\n%q", event)
	var elements []string
	for line := range strings.Lines(body) {
		field, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "data: ")
		require.True(t, ok, "not a data field: %q in\n%q", line, event)
		if el, ok := strings.CutPrefix(field, "elements "); ok {
			elements = append(elements, el)
		}
	}
	return strings.Join(elements, "\n")
}

// TestCarriageReturnInContent tests that CR cannot inject fields and is
// normalized like an HTML line break.
func TestCarriageReturnInContent(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		text, want string
	}{
		"lone":            {"a\rb", "a\nb"},
		"pair":            {"a\r\nb", "a\nb"},
		"before a pair":   {"a\r\r\nb", "a\n\nb"},
		"after a newline": {"a\n\rb", "a\n\nb"},
		"at the end":      {"a\r", "a\n"},
		"signals field": {
			"hi\revent: datastar-patch-signals\rdata: signals alert(1)\r",
			"hi\nevent: datastar-patch-signals\ndata: signals alert(1)\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			el := templ.Raw(`<p id="m">` + tc.text + `</p>`)
			want := `<p id="m">` + tc.want + `</p>`

			got := frame(t, func(s *sse.Stream) error { return s.PatchElement(el) })
			require.Equal(t, want, patchedElements(t, got))

			got = frame(t, func(s *sse.Stream) error {
				return s.PatchElementAt(el, "#t", datapages.PatchModeInner)
			})
			require.Equal(t, want, patchedElements(t, got))

			got = frame(t, func(s *sse.Stream) error { return s.ExecuteScript(tc.text) })
			require.Equal(t, `<script data-effect="el.remove()">`+tc.want+`</script>`,
				patchedElements(t, got))
		})
	}
}

// chunks renders in 2 writes to cover a CRLF split across writes.
func chunks(s string, i int) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		if _, err := w.Write([]byte(s[:i])); err != nil {
			return err
		}
		_, err := w.Write([]byte(s[i:]))
		return err
	})
}

// FuzzContentLineBreaks tests that arbitrary content stays within a single
// element field and normalizes each line break to LF.
func FuzzContentLineBreaks(f *testing.F) {
	for _, seed := range []struct {
		s     string
		split uint
	}{
		{"hi\revent: datastar-patch-signals\rdata: signals alert(1)\r", 3},
		{"a\r\nb\n\rc\r\r\nd", 2},
		{"\n\nevent: x\ndata: signals alert(1)\n\n", 1},
		{"", 0},
	} {
		f.Add(seed.s, seed.split)
	}
	withoutBreaks := strings.NewReplacer("\r", "", "\n", "")
	f.Fuzz(func(t *testing.T, s string, split uint) {
		i := int(split % uint(len(s)+1))
		for _, tc := range []struct {
			call func(x datapages.SSE) error
			sent string // The content the call puts in the patch.
		}{
			{
				call: func(x datapages.SSE) error {
					return x.PatchElement(templ.Raw(s))
				},
				sent: s,
			},
			{
				call: func(x datapages.SSE) error {
					return x.PatchElement(chunks(s, i))
				},
				sent: s,
			},
			{
				call: func(x datapages.SSE) error {
					return x.ExecuteScript(s)
				},
				sent: `<script data-effect="el.remove()">` + s + `</script>`,
			},
		} {
			el := patchedElements(t, frame(t, func(x *sse.Stream) error {
				return tc.call(x)
			}))
			require.Equal(t,
				withoutBreaks.Replace(tc.sent),
				strings.ReplaceAll(el, "\n", ""))
			breaks := strings.Count(tc.sent, "\r") + strings.Count(tc.sent, "\n") -
				strings.Count(tc.sent, "\r\n")
			require.Equal(t, breaks, strings.Count(el, "\n"))
		}
	})
}

// TestRedirectCarriesNoTag tests that a target cannot close the script element.
func TestRedirectCarriesNoTag(t *testing.T) {
	t.Parallel()

	got := frame(t, func(s *sse.Stream) error {
		return s.Redirect(`/x</script><img src=x onerror=alert(1)>`)
	})

	require.NotContains(t, got, "</script><img",
		"the target ended the script element:\n%s", got)
	require.Contains(t, got, `\u003c/script>`,
		"the target is not encoded:\n%s", got)
	require.Contains(t, got, "window.location.href")
}

// FuzzRedirect tests that arbitrary targets stay inside the string of the script.
// Valid UTF-8 targets round-trip unchanged.
func FuzzRedirect(f *testing.F) {
	for _, seed := range []string{
		"/a/",
		`/x</script><img src=x onerror=alert(1)>`,
		"/u/<!--<script>",
		"/x\r\ny",
		`"\`,
		"\x00\x1f\x7f",
		"/\u00e4\u2028/",
		"\xff\xfe",
	} {
		f.Add(seed)
	}
	const head = `<script data-effect="el.remove()">setTimeout(() => window.location.href = `
	f.Fuzz(func(t *testing.T, target string) {
		el := patchedElements(t, frame(t, func(s *sse.Stream) error {
			return s.Redirect(target)
		}))
		lit, ok := strings.CutPrefix(el, head)
		require.True(t, ok, "not a redirect script:\n%s", el)
		lit, ok = strings.CutSuffix(lit, ")</script>")
		require.True(t, ok, "the target ended the script:\n%s", el)
		// Escaping every '<' prevents </script> and <!-- from changing the element.
		require.NotContains(t, lit, "<")
		require.NotContains(t, lit, "\n")
		var got string
		require.NoError(t, json.Unmarshal([]byte(lit), &got),
			"the target is not a string literal:\n%s", lit)
		if utf8.ValidString(target) {
			require.Equal(t, target, got)
		}
	})
}

func signalsOf(t *testing.T, event string) string {
	t.Helper()
	require.NotContains(t, event, "\r",
		"a carriage return reached the event stream:\n%q", event)
	var lines []string
	for line := range strings.Lines(event) {
		if s, ok := strings.CutPrefix(line, "data: signals "); ok {
			lines = append(lines, strings.TrimSuffix(s, "\n"))
		}
	}
	return strings.Join(lines, "\n")
}

// TestPatchSignals tests struct, raw and if-missing patches.
// Invalid values return an error without a partial event.
func TestPatchSignals(t *testing.T) {
	t.Parallel()

	type signals struct {
		Count int `json:"count"`
	}

	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			return s.PatchSignals(signals{Count: 7})
		})
		require.Equal(t, "event: datastar-patch-signals\n"+
			`data: signals {"count":7}`+"\n\n", got)
	})

	t.Run("raw json", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			return s.PatchSignals(json.RawMessage(`{"count":7}`))
		})
		require.Contains(t, got, `data: signals {"count":7}`)
	})

	t.Run("raw json with line breaks", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			return s.PatchSignals(json.RawMessage("{\r\n\"count\":\r7\n}"))
		})
		var v signals
		require.NoError(t, json.Unmarshal([]byte(signalsOf(t, got)), &v))
		require.Equal(t, signals{Count: 7}, v)
	})

	t.Run("if missing", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			return s.PatchSignalsIfMissing(signals{Count: 3})
		})
		require.Equal(t, "event: datastar-patch-signals\n"+
			"data: onlyIfMissing true\n"+
			`data: signals {"count":3}`+"\n\n", got)
	})

	t.Run("invalid raw json", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			require.Error(t, s.PatchSignals(json.RawMessage(`{"count":`)))
			return nil
		})
		require.Empty(t, got)
	})

	t.Run("value that does not marshal", func(t *testing.T) {
		t.Parallel()
		got := frame(t, func(s *sse.Stream) error {
			require.Error(t, s.PatchSignals(make(chan int)))
			return nil
		})
		require.Empty(t, got)
	})
}

// TestRetry tests events that set the reconnect delay.
func TestRetry(t *testing.T) {
	t.Parallel()

	got := frame(t, func(s *sse.Stream) error {
		return s.DelayReconnect(5 * time.Second)
	})
	require.Equal(t, "event: datastar-patch-signals\n"+
		"retry: 5000\n"+
		"data: signals {}\n\n", got)

	got = frame(t, func(s *sse.Stream) error {
		return s.ExecuteScriptRetry("location.reload()", 5*time.Second)
	})
	require.Equal(t, "event: datastar-patch-elements\n"+
		"retry: 5000\n"+
		"data: selector body\n"+
		"data: mode append\n"+
		`data: elements <script data-effect="el.remove()">location.reload()</script>`+
		"\n\n", got)
}

// TestScriptAndRedirect tests script, redirect and prefetch events.
func TestScriptAndRedirect(t *testing.T) {
	t.Parallel()

	got := frame(t, func(s *sse.Stream) error {
		return s.ExecuteScript(`console.log("x")`)
	})
	require.Equal(t, "event: datastar-patch-elements\n"+
		"data: selector body\n"+
		"data: mode append\n"+
		`data: elements <script data-effect="el.remove()">console.log("x")</script>`+
		"\n\n", got)

	got = frame(t, func(s *sse.Stream) error {
		return s.Redirect("/next/")
	})
	require.Contains(t, got,
		`data: elements <script data-effect="el.remove()">`+
			`setTimeout(() => window.location.href = "/next/")</script>`)

	got = frame(t, func(s *sse.Stream) error {
		return s.Prefetch("/a/", "/b/")
	})
	require.Contains(t, got, "/a/")
	require.Contains(t, got, "/b/")
}

// TestConcurrentWritersAndClose tests atomic sends during concurrent Close.
func TestConcurrentWritersAndClose(t *testing.T) {
	t.Parallel()

	for _, coding := range append([]string{"identity"}, codings...) {
		t.Run(coding, func(t *testing.T) {
			t.Parallel()
			for range 5 {
				w := &recorder{h: make(http.Header)}
				s := sse.Open(w, codingRequest(coding), compression)
				var lock sync.Mutex
				sent := make(map[string]bool)
				tenth := make(chan struct{})
				var wg sync.WaitGroup
				for g := range 8 {
					wg.Go(func() {
						for i := range 20 {
							id := fmt.Sprintf("g%di%d", g, i)
							// Up to 100 KB, which spans 2 frames.
							size := (g*997 + i*7919) % (100 << 10)
							body := `<p id="` + id + `">` + strings.Repeat("y", size) +
								"\r\nend</p>"
							if s.PatchElement(templ.Raw(body)) == nil {
								lock.Lock()
								sent[id] = true
								if len(sent) == 10 {
									close(tenth)
								}
								lock.Unlock()
							}
						}
					})
				}
				wg.Go(func() {
					<-tenth
					s.Close()
				})
				wg.Wait()

				out := w.buf.String()
				if coding != "identity" {
					out = decode(t, coding, w.buf.Bytes())
				}
				events := strings.SplitAfter(out, "\n\n")
				require.Empty(t, events[len(events)-1], "the stream ends mid-event")
				events = events[:len(events)-1]
				require.Len(t, events, len(sent))
				for _, e := range events {
					head, rest, ok := strings.Cut(e,
						"\ndata: elements end</p>\n\n")
					require.True(t, ok, "a mangled event: %.80q", e)
					require.Empty(t, rest)
					id, _, _ := strings.Cut(strings.TrimPrefix(head,
						"event: datastar-patch-elements\ndata: elements <p id=\""), `"`)
					require.True(t, sent[id], "event %q arrived, but its send failed",
						id)
				}
			}
		})
	}
}

// TestClose tests repeated Close calls and an event after Close.
func TestClose(t *testing.T) {
	t.Parallel()

	w := &recorder{h: make(http.Header)}
	s := sse.Open(w, streamRequest(), compression)
	s.Close()
	require.Error(t, s.PatchElement(element))
	s.Close()
	require.NoError(t, s.Err())
	require.Zero(t, w.buf.Len())
}

// TestWriteFails tests that later events return the first write error.
func TestWriteFails(t *testing.T) {
	t.Parallel()

	for _, coding := range append([]string{"identity"}, codings...) {
		t.Run(coding, func(t *testing.T) {
			t.Parallel()
			r := codingRequest(coding)
			errGone := errors.New("client gone")
			w := &recorder{h: make(http.Header)}
			s := sse.Open(w, r, compression)
			w.err = errGone
			require.ErrorIs(t, s.PatchElement(element), errGone)
			require.ErrorIs(t, s.PatchElement(element), errGone)
			require.ErrorIs(t, s.Err(), errGone)
			s.Close()
			require.ErrorIs(t, s.Err(), errGone)
		})
	}
}

// TestCanceledContext tests an event after request cancellation.
func TestCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	w := &recorder{h: make(http.Header)}
	s := sse.Open(w, streamRequest().WithContext(ctx), compression)
	cancel()
	require.ErrorIs(t, s.PatchElement(element), context.Canceled)
	s.Close()
	require.Zero(t, w.buf.Len())
}

// TestCompressed tests that each encoder flushes events and writes its trailer.
func TestCompressed(t *testing.T) {
	t.Parallel()

	events := frame(t, func(s *sse.Stream) error {
		return s.PatchElement(element)
	}) +
		frame(t, func(s *sse.Stream) error {
			return s.PatchSignals(map[string]int{"n": 1})
		})
	for _, coding := range codings {
		t.Run(coding, func(t *testing.T) {
			t.Parallel()
			w := &recorder{h: make(http.Header)}
			s := sse.Open(w, codingRequest(coding), compression)
			require.Equal(t, coding, w.h.Get("Content-Encoding"))

			require.NoError(t, s.PatchElement(element))
			require.NoError(t, s.PatchSignals(map[string]int{"n": 1}))
			got := make([]byte, len(events))
			_, err := io.ReadFull(decoder(t, coding, w.buf.Bytes()), got)
			require.NoError(t, err, "an event waits in the encoder")
			require.Equal(t, events, string(got))

			s.Close()
			require.NoError(t, s.Err())
			require.Equal(t, events, decode(t, coding, w.buf.Bytes()))
		})
	}
}

// TestCompressedWithoutEvents tests a valid empty body for each coding.
func TestCompressedWithoutEvents(t *testing.T) {
	t.Parallel()

	for _, coding := range codings {
		t.Run(coding, func(t *testing.T) {
			t.Parallel()
			w := &recorder{h: make(http.Header)}
			s := sse.Open(w, codingRequest(coding), compression)
			s.Close()
			require.NoError(t, s.Err())
			require.NotZero(t, w.buf.Len())
			require.Empty(t, decode(t, coding, w.buf.Bytes()))
		})
	}
}

// TestCompressionConfig tests negotiation and decoding for each configuration.
func TestCompressionConfig(t *testing.T) {
	t.Parallel()

	const browser = "gzip, deflate, br, zstd"
	for name, tc := range map[string]struct {
		conf           datapages.SSECompressionConfig
		acceptEncoding string
		want           string // "" for no compression.
	}{
		"disabled": {
			conf:           datapages.SSECompressionConfig{Disabled: true},
			acceptEncoding: browser,
		},
		"gzip preferred": {
			conf: datapages.SSECompressionConfig{Codings: []datapages.SSECoding{
				datapages.SSECodingGzip, datapages.SSECodingZstd,
			}},
			acceptEncoding: browser, want: "gzip",
		},
		"zstd only, client without zstd": {
			conf: datapages.SSECompressionConfig{Codings: []datapages.SSECoding{
				datapages.SSECodingZstd,
			}},
			acceptEncoding: "gzip, br",
		},
		"zstd smallest window": {
			conf: datapages.SSECompressionConfig{
				ZstdLevel: 22, ZstdWindow: 1 << 10,
			},
			acceptEncoding: "zstd", want: "zstd",
		},
		"zstd largest window": {
			conf: datapages.SSECompressionConfig{
				ZstdLevel: 1, ZstdWindow: 8 << 20,
			},
			acceptEncoding: "zstd", want: "zstd",
		},
		"brotli smallest window": {
			conf: datapages.SSECompressionConfig{
				BrotliQuality: 11, BrotliWindow: 1 << 10,
			},
			acceptEncoding: "br", want: "br",
		},
		"brotli largest window": {
			conf: datapages.SSECompressionConfig{
				BrotliQuality: 1, BrotliWindow: 16 << 20,
			},
			acceptEncoding: "br", want: "br",
		},
		"gzip best": {
			conf:           datapages.SSECompressionConfig{GzipLevel: 9},
			acceptEncoding: "gzip", want: "gzip",
		},
		"gzip fastest": {
			conf:           datapages.SSECompressionConfig{GzipLevel: 1},
			acceptEncoding: "gzip", want: "gzip",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opt := datapages.WithSSECompression(tc.conf)
			var cfg datapages.ServerConfig
			require.NoError(t, opt(&cfg))

			el := templ.Raw(`<ul id="l">` +
				strings.Repeat("\n<li>item</li>", 300) +
				"\n</ul>")
			want := frame(t, func(s *sse.Stream) error { return s.PatchElement(el) })
			w := &recorder{h: make(http.Header)}
			s := sse.Open(w,
				codingRequest(tc.acceptEncoding),
				sse.NewCompression(cfg.SSECompression))
			require.NoError(t, s.PatchElement(el))
			require.NoError(t, s.PatchElement(el))
			s.Close()
			require.NoError(t, s.Err())

			require.Equal(t, tc.want, w.h.Get("Content-Encoding"))
			if tc.want == "" {
				require.Equal(t, want+want, w.buf.String())
				return
			}
			require.Equal(t, want+want, decode(t, tc.want, w.buf.Bytes()))
		})
	}
}

// TestOpenWithoutCompression tests a stream of a server that compresses nothing.
// Its response doesn't vary with Accept-Encoding.
func TestOpenWithoutCompression(t *testing.T) {
	t.Parallel()

	w := &recorder{h: make(http.Header)}
	s := sse.Open(w, codingRequest("gzip, deflate, br, zstd"), nil)
	require.NoError(t, s.PatchElement(element))
	s.Close()
	require.Empty(t, w.h.Get("Content-Encoding"))
	require.Empty(t, w.h.Values("Vary"))
	require.Equal(t, "event: datastar-patch-elements\n"+
		`data: elements <div id="out">x</div>`+"\n\n", w.buf.String())
}

// TestReadSignals tests query and body signal sources.
func TestReadSignals(t *testing.T) {
	t.Parallel()

	type signals struct {
		Count int `json:"count"`
	}
	for name, tc := range map[string]struct {
		method, url, body string
		want              signals
		wantErr           bool
	}{
		"get": {
			method: http.MethodGet,
			url:    "/?datastar=%7B%22count%22%3A2%7D",
			want:   signals{Count: 2},
		},
		"get without": {
			method: http.MethodGet,
			url:    "/?other=1",
			want:   signals{Count: 1},
		},
		"get empty": {
			method: http.MethodGet,
			url:    "/?datastar=",
			want:   signals{Count: 1},
		},
		"delete": {
			method: http.MethodDelete,
			url:    "/?datastar=%7B%22count%22%3A3%7D",
			want:   signals{Count: 3},
		},
		"get with a body": {
			method: http.MethodGet,
			url:    "/", body: `{"count":9}`,
			want: signals{Count: 1},
		},
		"post": {
			method: http.MethodPost,
			url:    "/", body: `{"count":4}`,
			want: signals{Count: 4},
		},
		"query method": {
			method: "QUERY",
			url:    "/", body: `{"count":5}`,
			want: signals{Count: 5},
		},
		"post ignores query": {
			method: http.MethodPost,
			url:    "/?datastar=%7B%7D", body: `{"count":6}`,
			want: signals{Count: 6},
		},
		"get invalid": {
			method: http.MethodGet,
			url:    "/?datastar=%7B", wantErr: true,
		},
		"post invalid": {
			method: http.MethodPost,
			url:    "/", body: `{"count":`, wantErr: true,
		},
		"post empty": {
			method: http.MethodPost,
			url:    "/", wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
			v := signals{Count: 1}
			err := sse.ReadSignals(r, &v)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, v)
		})
	}
}

// TestPatchElementAllocs tests warm pooled writes with every coding,
// of an event in 1 frame and of 1 that spans 4.
func TestPatchElementAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes sync.Pool drop values")
	}
	for name, items := range map[string]int{"small": 200, "large": 8000} {
		el := templ.Raw(`<ul id="l">` +
			strings.Repeat("\n<li>item</li>", items) +
			"\n</ul>")
		for _, coding := range append([]string{"identity"}, codings...) {
			t.Run(name+"/"+coding, func(t *testing.T) {
				s := sse.Open(
					&discard{h: make(http.Header)},
					codingRequest(coding),
					compression,
				)
				defer s.Close()
				require.NoError(t, s.PatchElement(el))
				require.Zero(t, testing.AllocsPerRun(100, func() {
					if err := s.PatchElement(el); err != nil {
						t.Fatal(err)
					}
				}))
			})
		}
	}
}

// TestScriptAllocs tests that Redirect and Prefetch write into the event
// without allocating, also when a URL needs escaping.
func TestScriptAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes sync.Pool drop values")
	}
	urls := []string{"/listing/42/", "/listing/43/", "/search/?q=bike&page=2"}
	escaped := []string{`/x"</script>`, "/a\\b\x00"}
	for name, call := range map[string]func(s *sse.Stream) error{
		"redirect":         func(s *sse.Stream) error { return s.Redirect(urls[2]) },
		"redirect escaped": func(s *sse.Stream) error { return s.Redirect(escaped[0]) },
		"prefetch":         func(s *sse.Stream) error { return s.Prefetch(urls...) },
		"prefetch escaped": func(s *sse.Stream) error { return s.Prefetch(escaped...) },
	} {
		t.Run(name, func(t *testing.T) {
			s := sse.Open(&discard{h: make(http.Header)}, streamRequest(), compression)
			defer s.Close()
			require.NoError(t, call(s))
			require.Zero(t, testing.AllocsPerRun(100, func() {
				if err := call(s); err != nil {
					t.Fatal(err)
				}
			}))
		})
	}
}

// TestLargeEvents tests frame reuse between events up to 3 MiB.
func TestLargeEvents(t *testing.T) {
	t.Parallel()

	content := func(size int, fill byte) string {
		line := strings.Repeat(string(fill), 99) + "\n"
		return strings.Repeat(line, size/len(line)+1)[:size]
	}
	errRender := errors.New("render failed")
	for name, component := range map[string]func(s string) templ.Component{
		"1 write": func(s string) templ.Component { return templ.Raw(s) },
		"2 writes": func(s string) templ.Component {
			return chunks(s, len(s)/2)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for i, size := range []int{
				100, 70 << 10, 300 << 10, 2 << 20, 50, 100 << 10,
				64 << 10, 64<<10 + 1, 3 << 20, 10,
			} {
				s := content(size, byte('a'+i))
				got := frame(t, func(x *sse.Stream) error {
					err := x.PatchElement(templ.ComponentFunc(
						func(_ context.Context, w io.Writer) error {
							_, _ = io.WriteString(w, s)
							return errRender
						},
					))
					require.ErrorIs(t, err, errRender)
					return x.PatchElement(component(s))
				})
				require.Equal(t, s, patchedElements(t, got),
					"event %d of %d bytes", i, size)
			}
		})
	}
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
			got := frame(t, func(s *sse.Stream) error {
				return s.Prefetch("/a/", url)
			})
			require.Equal(t, []string{"/a/", url}, prefetchURLs(t, got))
		})
	}
}

func TestPrefetchWithoutURLs(t *testing.T) {
	t.Parallel()

	got := frame(t, func(s *sse.Stream) error { return s.Prefetch() })
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
		got := frame(t, func(s *sse.Stream) error { return s.Prefetch(a, b) })
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

// BenchmarkPatchElement measures 4 element sizes with every coding.
func BenchmarkPatchElement(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{
		{"200B", 200},
		{"8KiB", 8 << 10},
		{"100KiB", 100 << 10},
		{"1MiB", 1 << 20},
	} {
		el := templ.Raw(`<ul id="l">` +
			strings.Repeat("<li>item text</li>\n", size.n/19) +
			`</ul>`)
		for _, coding := range append([]string{"identity"}, codings...) {
			b.Run(size.name+"/"+coding, func(b *testing.B) {
				s := sse.Open(
					&discard{h: make(http.Header)},
					codingRequest(coding),
					compression,
				)
				defer s.Close()
				b.ReportAllocs()
				for b.Loop() {
					if err := s.PatchElement(el); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkReRender measures distinct list re-renders, wire bytes and retained heap.
func BenchmarkReRender(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{
		{"8KB", 8_000},
		{"100KB", 100_000},
		{"400KB", 400_000},
		{"2MB", 2_000_000},
	} {
		items := listItems(size.n)
		lowMemory := datapages.SSECompressionLowMemory()
		lowBandwidth := datapages.SSECompressionLowBandwidth()
		largeFragments := datapages.SSECompressionLargeFragments()
		const browser = "gzip, deflate, br, zstd"
		for _, tc := range []struct {
			name, coding string
			conf         *datapages.SSECompressionConfig
		}{
			{"zstd", "zstd", nil},
			{"br", "br", nil},
			{"gzip", "gzip", nil},
			{"zstd-4MiB", "zstd", &datapages.SSECompressionConfig{ZstdWindow: 4 << 20}},
			{"zstd-level10", "zstd", &datapages.SSECompressionConfig{ZstdLevel: 10}},
			{"br-quality9", "br", &datapages.SSECompressionConfig{BrotliQuality: 9}},
			{"low-memory", browser, &lowMemory},
			{"low-memory-br", "br", &lowMemory},
			{"low-bandwidth", browser, &lowBandwidth},
			{"large-fragments", browser, &largeFragments},
			{"large-fragments-br", "br", &largeFragments},
		} {
			b.Run(size.name+"/"+tc.name, func(b *testing.B) {
				comp := sse.NewCompression(tc.conf)
				w := &countingWriter{discard: discard{h: make(http.Header)}}
				versions := make([]int, len(items))
				// 2 collections empty the pools of the previous runs.
				var before, after runtime.MemStats
				runtime.GC()
				runtime.GC()
				runtime.ReadMemStats(&before)
				s := sse.Open(w, codingRequest(tc.coding), comp)
				defer s.Close()
				// The 1st render has no earlier one to refer to.
				if err := s.PatchElement(listRender(items, versions)); err != nil {
					b.Fatal(err)
				}
				first, renders := w.n, 0
				for b.Loop() {
					renders++
					versions[renders%len(items)] = renders
					if err := s.PatchElement(listRender(items, versions)); err != nil {
						b.Fatal(err)
					}
				}
				runtime.GC()
				runtime.GC()
				runtime.ReadMemStats(&after)
				perRender := float64(w.n-first) / float64(renders)
				b.ReportMetric(perRender, "wireB/op")
				b.ReportMetric(perRender/float64(size.n)*100, "%raw")
				b.ReportMetric(
					float64(after.HeapAlloc-before.HeapAlloc)/(1<<20), "heapMiB",
				)
			})
		}
	}
}

// countingWriter counts the bytes of a response and discards them.
type countingWriter struct {
	discard
	n int
}

func (c *countingWriter) Write(b []byte) (int, error) {
	c.n += len(b)
	return len(b), nil
}

// listItems returns the items of a list of about n bytes.
// Their words come from a fixed seed, which makes the items differ as real ones do.
func listItems(n int) []string {
	r := rand.New(rand.NewPCG(3, 4))
	words := strings.Fields("alpha bravo charlie delta echo foxtrot golf " +
		"hotel india juliet kilo lima mike november oscar papa quebec romeo " +
		"sierra tango uniform victor whiskey xray yankee zulu")
	var items []string
	for size := 0; size < n; {
		var b strings.Builder
		b.WriteString(`<li id="item-` + strconv.Itoa(len(items)) +
			`"><input type="checkbox">`)
		for range 3 + r.IntN(4) {
			b.WriteString(" " + words[r.IntN(len(words))])
		}
		b.WriteString("</li>")
		items = append(items, b.String())
		size += b.Len()
	}
	return items
}

// listRender renders the list of items. An item with a version other than 0
// shows the version instead of its text.
func listRender(items []string, versions []int) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, _ = io.WriteString(w, `<ul id="list">`)
		for i, it := range items {
			if versions[i] == 0 {
				_, _ = io.WriteString(w, it)
				continue
			}
			_, _ = io.WriteString(w, `<li id="item-`+strconv.Itoa(i)+
				`"><input type="checkbox" checked> changed in render `+
				strconv.Itoa(versions[i])+`</li>`)
		}
		_, err := io.WriteString(w, `</ul>`)
		return err
	})
}

// BenchmarkHTTPEvent measures frame writes over cleartext and TLS HTTP/1.1.
func BenchmarkHTTPEvent(b *testing.B) {
	for name, el := range map[string]templ.Component{
		"100KiB-lines": templ.Raw(strings.Repeat("<li>item text</li>\n", (100<<10)/19)),
		"2MB-1line":    templ.Raw(strings.Repeat("x", 2_000_000)),
	} {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := sse.Open(w, r, nil)
			defer s.Close()
			_ = s.PatchElement(el)
		})
		for _, scheme := range []string{"http", "https"} {
			b.Run(name+"/"+scheme, func(b *testing.B) {
				srv := httptest.NewUnstartedServer(h)
				if scheme == "https" {
					srv.StartTLS()
				} else {
					srv.Start()
				}
				defer srv.Close()
				client := srv.Client()
				for b.Loop() {
					resp, err := client.Get(srv.URL)
					if err != nil {
						b.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			})
		}
	}
}

// BenchmarkRedirect measures a redirect to an href-generated URL.
func BenchmarkRedirect(b *testing.B) {
	s := sse.Open(&discard{h: make(http.Header)}, streamRequest(), compression)
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Redirect("/search/?q=bike&page=2"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrefetch measures the href-generated URL case.
func BenchmarkPrefetch(b *testing.B) {
	s := sse.Open(&discard{h: make(http.Header)}, streamRequest(), compression)
	urls := []string{"/listing/42/", "/listing/43/", "/search/?q=bike&page=2"}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Prefetch(urls...); err != nil {
			b.Fatal(err)
		}
	}
}
