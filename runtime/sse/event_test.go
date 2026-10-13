package sse

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEventFraming tests random element patches against a reference encoder.
// Their content holds CR, LF and CRLF, arrives in writes of random sizes,
// empty and 1-byte writes included, and spans several frames, as their fields do.
func TestEventFraming(t *testing.T) {
	t.Parallel()

	const prefix = "data: elements "
	r := rand.New(rand.NewPCG(1, 2))
	for i := range 300 {
		content := randomContent(r)
		var retry time.Duration
		if r.IntN(3) == 0 {
			retry = time.Duration(r.IntN(100_000)) * time.Millisecond
		}
		var fields [][2]string
		if r.IntN(2) == 0 {
			fields = append(fields,
				[2]string{"selector ", "#" + strings.Repeat("x", r.IntN(70<<10))})
		}
		if r.IntN(2) == 0 {
			fields = append(fields, [2]string{"mode ", "append"})
		}

		e := newEvent(eventPatchElements)
		e.retry(retry)
		for _, f := range fields {
			e.field(f[0], f[1])
		}
		e.prefix = prefix
		for _, w := range randomSplit(r, content) {
			var n int
			if r.IntN(2) == 0 {
				n, _ = e.Write([]byte(w))
			} else {
				n, _ = e.WriteString(w)
			}
			require.Equal(t, len(w), n)
		}
		e.end()
		for _, f := range e.full {
			require.Len(t, f, frameSize, "event %d has a frame that is not full", i)
		}
		var b strings.Builder
		require.NoError(t, e.writeFrames(&b))
		e.free()

		got, want := b.String(), referenceEvent(retry, fields, prefix, content)
		if got != want {
			d := 0
			for d < len(got) && d < len(want) && got[d] == want[d] {
				d++
			}
			lo := max(0, d-40)
			require.Equal(t, want[lo:min(len(want), d+40)], got[lo:min(len(got), d+40)],
				"event %d differs at byte %d of %d", i, d, len(want))
		}
	}
}

// referenceEvent returns the wire form of an element patch. Its fields come first,
// then a data line for each line of content, which CR, LF and CRLF end.
// Content that was never written gets no data line.
func referenceEvent(
	retry time.Duration, fields [][2]string, prefix, content string,
) string {
	var b strings.Builder
	b.WriteString("event: " + eventPatchElements + "\n")
	if retry > 0 {
		b.WriteString("retry: " + strconv.FormatInt(retry.Milliseconds(), 10) + "\n")
	}
	for _, f := range fields {
		b.WriteString("data: " + f[0] + f[1] + "\n")
	}
	if content != "" {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\r", "\n")
		for line := range strings.SplitSeq(content, "\n") {
			b.WriteString(prefix + line + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// randomContent returns up to 300 KiB of letters with CR and LF at random places.
// 1 in 10 is made of CR and LF only.
func randomContent(r *rand.Rand) string {
	var b []byte
	switch r.IntN(4) {
	case 0:
		b = make([]byte, r.IntN(16))
	case 1:
		b = make([]byte, r.IntN(600))
	case 2:
		b = make([]byte, r.IntN(70<<10))
	default:
		b = make([]byte, r.IntN(300<<10))
	}
	breaksOnly := r.IntN(10) == 0
	for i := range b {
		switch x := r.IntN(1000); {
		case breaksOnly:
			b[i] = "\r\n"[r.IntN(2)]
		case x < 3:
			b[i] = '\r'
		case x < 6:
			b[i] = '\n'
		default:
			b[i] = 'a' + byte(r.IntN(26))
		}
	}
	return string(b)
}

// randomSplit splits s into writes of random sizes, empty ones included.
func randomSplit(r *rand.Rand, s string) []string {
	var writes []string
	for s != "" {
		var n int
		switch r.IntN(5) {
		case 0:
		case 1:
			n = 1
		case 2:
			n = r.IntN(8)
		case 3:
			n = r.IntN(5000)
		default:
			n = len(s)
		}
		n = min(n, len(s))
		writes = append(writes, s[:n])
		s = s[n:]
	}
	if r.IntN(10) == 0 {
		writes = append(writes, "")
	}
	return writes
}
