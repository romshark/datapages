package sse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var urlsWithoutEscapes = map[string][]string{
	"one":   {"/listing/42/"},
	"three": {"/listing/42/", "/listing/43/", "/search/?q=bike&page=2"},
	"empty": {""},
}

// TestSpeculationRulesLen tests exact sizing for URLs that need no escaping.
// Allocator size classes can hide short reallocations from [testing.AllocsPerRun].
func TestSpeculationRulesLen(t *testing.T) {
	for name, urls := range urlsWithoutEscapes {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, len(speculationRules(urls)), speculationRulesLen(urls))
		})
	}
}

// TestSpeculationRulesUnescapedAllocatesOnce tests the no-escaping fast path.
func TestSpeculationRulesUnescapedAllocatesOnce(t *testing.T) {
	for name, urls := range urlsWithoutEscapes {
		t.Run(name, func(t *testing.T) {
			n := testing.AllocsPerRun(100, func() { _ = speculationRules(urls) })
			require.Equal(t, 1.0, n)
		})
	}
}
