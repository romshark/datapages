//go:build race

package sse_test

// raceEnabled is true under the race detector, where [sync.Pool.Put] drops
// 1 in 4 values and pooled writes allocate again.
const raceEnabled = true
