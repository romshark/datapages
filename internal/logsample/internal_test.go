package logsample

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTrackCountsALostStore tests that a first record whose
// key another one stored first is counted as suppressed.
func TestTrackCountsALostStore(t *testing.T) {
	var s state
	require.True(t, s.track("k", 0, DefaultLimit, time.Minute))
	require.False(t, s.track("k", 0, DefaultLimit, time.Minute))

	v, ok := s.seen.Load("k")
	require.True(t, ok)
	require.Equal(t, int64(1), v.(*entry).suppressed.Load())
}
