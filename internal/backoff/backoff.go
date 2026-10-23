// Package backoff spaces retries with growing pauses.
// The randomness in each pause spreads the retries of callers that fail together.
package backoff

import (
	"iter"
	"math/rand/v2"
	"time"
)

// Attempts yields 1 to n with pauses growing from first by factor, each cut by
// up to 1/2 at random. first must be positive, factor at least 1.
func Attempts(n int, first time.Duration, factor float64) iter.Seq[int] {
	return func(yield func(int) bool) {
		pause := first
		for attempt := 1; attempt <= n; attempt++ {
			if attempt > 1 {
				time.Sleep(pause/2 + rand.N(pause-pause/2))
				pause = time.Duration(float64(pause) * factor)
			}
			if !yield(attempt) {
				return
			}
		}
	}
}
