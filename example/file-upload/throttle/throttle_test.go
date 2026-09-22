package throttle_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/file-upload/throttle"
)

// Every duration bound in this file is wide on purpose: a test that pins a
// duration closely fails on a loaded machine without anything being wrong.

// TestUnlimited tests that a zero rate charges nothing.
func TestUnlimited(t *testing.T) {
	src := bytes.NewReader(make([]byte, 4<<20))
	r := throttle.Reader(context.Background(), src, throttle.NewLimiter(0))

	start := time.Now()
	n, err := io.Copy(io.Discard, r)
	require.NoError(t, err)
	require.Equal(t, int64(4<<20), n)
	require.Less(t, time.Since(start), time.Second,
		"an unlimited reader waited")
}

// TestRateIsEnforced tests that reading more than the budget takes the time
// the budget says, and that the whole content still arrives.
func TestRateIsEnforced(t *testing.T) {
	const rate = 512 << 10 // 512 KiB/s
	const size = 256 << 10 // half a second of it

	want := bytes.Repeat([]byte("datapages"), size/9)
	src := bytes.NewReader(want)
	r := throttle.Reader(context.Background(), src, throttle.NewLimiter(rate))

	start := time.Now()
	got, err := io.ReadAll(r)
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, want, got, "the throttled reader changed the content")

	// The reader starts with a quarter second of budget, which leaves at
	// least a quarter second of waiting for half a second of content.
	require.Greater(t, elapsed, 200*time.Millisecond,
		"%d bytes at %d bytes/s passed in %s", len(want), rate, elapsed)
}

// TestSharedBudget tests that two transfers on one limiter split its rate
// instead of getting it each.
func TestSharedBudget(t *testing.T) {
	const rate = 512 << 10
	const size = 128 << 10

	l := throttle.NewLimiter(rate)
	start := time.Now()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := bytes.NewReader(make([]byte, size))
			_, err := io.Copy(io.Discard, throttle.Reader(context.Background(), src, l))
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	// Four transfers of a quarter second of budget each are a second of it.
	require.Greater(t, time.Since(start), 500*time.Millisecond,
		"four transfers of %d bytes each ran faster than %d bytes/s allows",
		size, rate)
}

// TestSetRate tests that a limit set while a transfer runs reaches it.
func TestSetRate(t *testing.T) {
	l := throttle.NewLimiter(1 << 10)
	require.Equal(t, int64(1<<10), l.Rate())

	src := bytes.NewReader(make([]byte, 1<<20))
	r := throttle.Reader(context.Background(), src, l)

	// One megabyte at a kilobyte per second would take a quarter of an hour.
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.SetRate(0)
	}()
	start := time.Now()
	_, err := io.Copy(io.Discard, r)
	require.NoError(t, err)
	require.Less(t, time.Since(start), 10*time.Second,
		"lifting the limit did not reach the running transfer")

	l.SetRate(-1)
	require.Zero(t, l.Rate(), "a negative rate is not unlimited")
}

// TestContextEnds tests that a transfer whose request is gone stops waiting.
func TestContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := bytes.NewReader(make([]byte, 1<<20))
	r := throttle.Reader(ctx, src, throttle.NewLimiter(1<<10))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := io.Copy(io.Discard, r)
	require.ErrorIs(t, err, context.Canceled)
}
