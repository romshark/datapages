package backoff_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/backoff"
)

func TestAttempts(t *testing.T) {
	t.Parallel()

	for name, factor := range map[string]float64{
		"constant": 1,
		"1.5":      1.5,
		"doubling": 2,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var attempts []int
				var at []time.Time
				for attempt := range backoff.Attempts(5, time.Second, factor) {
					attempts = append(attempts, attempt)
					at = append(at, time.Now())
				}
				require.Zero(t, time.Since(at[len(at)-1]), "pause after the last attempt")
				require.Equal(t, []int{1, 2, 3, 4, 5}, attempts)
				pause := time.Second
				for i := 1; i < len(at); i++ {
					got := at[i].Sub(at[i-1])
					require.GreaterOrEqual(t, got, pause/2, "pause %d", i)
					require.Less(t, got, pause, "pause %d", i)
					pause = time.Duration(float64(pause) * factor)
				}
			})
		})
	}
}

// TestAttemptsBreak tests that a loop that breaks ends without another pause.
func TestAttemptsBreak(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var last time.Time
		for attempt := range backoff.Attempts(5, time.Second, 2) {
			last = time.Now()
			if attempt == 2 {
				break
			}
		}
		require.Zero(t, time.Since(last))
	})
}
