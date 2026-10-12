// Package testlog captures logs for exact comparison in tests.
package testlog

import (
	"bytes"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Buffer holds log records as text lines without the time.
// Records below [slog.LevelInfo] are dropped.
// It's safe for concurrent use. The zero value is ready to use.
type Buffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

// Handler returns a handler that writes to b.
func (b *Buffer) Handler() slog.Handler {
	return slog.NewTextHandler(b, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
}

// Logger returns a logger that writes to b.
func (b *Buffer) Logger() *slog.Logger { return slog.New(b.Handler()) }

// Write appends p.
func (b *Buffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.Write(p)
}

// Lines returns 1 line per record, or nil if nothing was logged.
// slog quotes line feeds in values, which keeps a record on 1 line.
func (b *Buffer) Lines() []string {
	b.lock.Lock()
	defer b.lock.Unlock()
	var lines []string
	for line := range strings.Lines(b.buf.String()) {
		lines = append(lines, strings.TrimSuffix(line, "\n"))
	}
	return lines
}

// RequireLines fails t unless the log is exactly want.
// With no want, it requires that nothing was logged.
func (b *Buffer) RequireLines(t testing.TB, want ...string) {
	t.Helper()
	if got := b.Lines(); !slices.Equal(got, want) {
		t.Fatalf("log lines differ\ngot:\n%s\nwant:\n%s",
			indent(got), indent(want))
	}
}

// RequireLogged fails t unless line was logged, among any others.
func (b *Buffer) RequireLogged(t testing.TB, line string) {
	t.Helper()
	if !b.hasLineInPlace(func(l []byte) bool { return string(l) == line }) {
		t.Fatalf("not logged:\n  %s\nlog:\n%s", line, indent(b.Lines()))
	}
}

// RequireMatch fails t unless a logged line matches the regular expression.
func (b *Buffer) RequireMatch(t testing.TB, pattern string) {
	t.Helper()
	if !b.hasLineInPlace(regexp.MustCompile(pattern).Match) {
		t.Fatalf("no logged line matches %s\nlog:\n%s", pattern, indent(b.Lines()))
	}
}

// AwaitMatch is [Buffer.RequireMatch] for a record logged up to timeout after
// the call, such as by a server goroutine that outlives the response.
func (b *Buffer) AwaitMatch(t testing.TB, pattern string, timeout time.Duration) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	deadline := time.Now().Add(timeout)
	for !b.hasLineInPlace(re.Match) {
		if time.Now().After(deadline) {
			t.Fatalf("no line matching %s logged within %v\nlog:\n%s",
				pattern, timeout, indent(b.Lines()))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// RequireBelow fails t if a record at level or above was logged.
// To require that nothing went wrong:
//
//	log.RequireBelow(t, slog.LevelWarn)
func (b *Buffer) RequireBelow(t testing.TB, level slog.Level) {
	t.Helper()
	if b.hasLineInPlace(func(line []byte) bool {
		return !levelBelow(line, level)
	}) {
		t.Fatalf("logged at %v or above:\n%s", level, indent(b.Lines()))
	}
}

// levelBelow reports whether line starts with a level below limit.
// A line without a level that slog can parse counts as not below.
func levelBelow(line []byte, limit slog.Level) bool {
	field, _, _ := bytes.Cut(line, []byte{' '})
	text, ok := bytes.CutPrefix(field, []byte("level="))
	var level slog.Level
	return ok && level.UnmarshalText(text) == nil && level < limit
}

func (b *Buffer) hasLineInPlace(match func(line []byte) bool) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	for line := range bytes.Lines(b.buf.Bytes()) {
		if match(bytes.TrimSuffix(line, lineFeed)) {
			return true
		}
	}
	return false
}

var lineFeed = []byte{'\n'}

// indent puts each line on its own indented line of a failure message.
func indent(lines []string) string {
	if len(lines) == 0 {
		return "  (none)"
	}
	return "  " + strings.Join(lines, "\n  ")
}
