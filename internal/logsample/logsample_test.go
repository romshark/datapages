package logsample_test

import (
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/romshark/datapages/internal/logsample"
	"github.com/romshark/datapages/internal/testlog"
)

const interval = time.Minute

// newLogger returns a logger and what it writes to. Every test here runs in a
// synctest bubble, where time.Sleep moves the clock the handler reads.
func newLogger(t *testing.T, limit int) (*slog.Logger, *testlog.Buffer) {
	t.Helper()
	var buf testlog.Buffer
	return slog.New(logsample.New(buf.Handler(), limit, interval)), &buf
}

// TestRepeatIsHeldBack tests the record a render path repeats. 1 line says
// what 1000 say, and the count of the rest rides on the next one.
func TestRepeatIsHeldBack(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)
		const line = "level=WARN msg=dropped value=x"

		for range 100 {
			log.Warn("dropped", slog.String("value", "x"))
		}
		buf.RequireLines(t, line)

		// Still happening after the interval, and the log says how often.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "x"))
		buf.RequireLines(t, line, line+" suppressed=99")

		// The count starts over.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "x"))
		buf.RequireLines(t, line, line+" suppressed=99", line)
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
		buf.RequireLines(t,
			"level=WARN msg=dropped value=x",
			"level=WARN msg=dropped value=y",
			"level=WARN msg=other value=x",
			"level=ERROR msg=dropped value=x",
		)
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
		buf.RequireLines(t,
			"level=WARN msg=dropped value=0",
			"level=WARN msg=dropped value=1",
			"level=WARN msg=dropped value=2",
		)
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
		buf.RequireLines(t,
			"level=WARN msg=dropped value=a",
			"level=WARN msg=dropped value=b",
		)

		// The first 2 have gone quiet, which makes room.
		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "c"))
		buf.RequireLines(t,
			"level=WARN msg=dropped value=a",
			"level=WARN msg=dropped value=b",
			"level=WARN msg=dropped value=c",
		)
	})
}

// TestConcurrentUse tests what every request path does to it at once.
// 1 record of the burst is written, and the next one counts the other 49.
func TestConcurrentUse(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		log, buf := newLogger(t, 0)
		const line = "level=WARN msg=dropped value=x"

		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				log.Warn("dropped", slog.String("value", "x"))
			})
		}
		wg.Wait()
		buf.RequireLines(t, line)

		time.Sleep(interval)
		log.Warn("dropped", slog.String("value", "x"))
		buf.RequireLines(t, line, line+" suppressed=49")
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
		buf.RequireLines(t,
			"level=WARN msg=dropped src=a",
			"level=WARN msg=dropped src=b",
		)
	})
}
