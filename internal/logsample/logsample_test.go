package logsample_test

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/logsample"
)

const interval = time.Minute

// newLogger returns a logger and what it writes to. Every test here runs in a
// synctest bubble, where time.Sleep moves the clock the handler reads.
func newLogger(t *testing.T, limit int) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, nil)
	return slog.New(logsample.New(h, limit, interval)), &buf
}

// TestRepeatIsHeldBack tests the record a render path repeats. One line says
// what a thousand say, and the count of the rest rides on the next one.
func TestRepeatIsHeldBack(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)

		for range 100 {
			log.Warn("dropped", slog.String("value", "x"))
		}
		require.Equal(t, 1, strings.Count(buf.String(), "dropped"))
		require.NotContains(t, buf.String(), "suppressed")

		// Still happening after the interval, and the log says how often.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "x"))
		require.Equal(t, 2, strings.Count(buf.String(), "dropped"))
		require.Contains(t, buf.String(), "suppressed=99")

		// The count starts over.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "x"))
		require.Equal(t, 1, strings.Count(buf.String(), "suppressed=99"))
	})
}

// TestDistinctRecordsPass tests that one mistake does not hide another.
func TestDistinctRecordsPass(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)

		log.Warn("dropped", slog.String("value", "x"))
		log.Warn("dropped", slog.String("value", "y"))
		log.Warn("other", slog.String("value", "x"))
		log.Error("dropped", slog.String("value", "x"))
		require.Equal(t, 4, strings.Count(buf.String(), "value=")) //nolint:mnd
	})
}

// TestLimitBoundsTheSet tests the cap on how many records are tracked.
// A value taken from a request would otherwise grow the set without end.
func TestLimitBoundsTheSet(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 3)

		for i := range 100 {
			log.Warn("dropped", slog.String("value", strconv.Itoa(i)))
		}
		require.Equal(t, 3, strings.Count(buf.String(), "dropped"))
	})
}

// TestFullSetRecovers tests a mistake found after the set filled up.
// Holding the first keys forever would leave it unreported.
func TestFullSetRecovers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 2)

		log.Warn("dropped", slog.String("value", "a"))
		log.Warn("dropped", slog.String("value", "b"))
		log.Warn("dropped", slog.String("value", "c"))
		require.Equal(t, 2, strings.Count(buf.String(), "dropped"))

		// The first two have gone quiet, which makes room.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "c"))
		require.Contains(t, buf.String(), "value=c")
	})
}

// TestConcurrentUse tests what every request path does to it at once.
func TestConcurrentUse(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)

		var wg sync.WaitGroup
		for range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Warn("dropped", slog.String("value", "x"))
			}()
		}
		wg.Wait()
		require.Equal(t, 1, strings.Count(buf.String(), "dropped"))
	})
}

// TestWithAttrsSharesTheSet tests a logger derived with With. Its records are
// counted against the same cap, and its attributes make a record distinct.
func TestWithAttrsSharesTheSet(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)

		a := log.With(slog.String("src", "a"))
		b := log.With(slog.String("src", "b"))
		for range 10 {
			a.Warn("dropped")
			b.Warn("dropped")
		}
		require.Equal(t, 2, strings.Count(buf.String(), "dropped"))
		require.Contains(t, buf.String(), "src=a")
		require.Contains(t, buf.String(), "src=b")
	})
}
