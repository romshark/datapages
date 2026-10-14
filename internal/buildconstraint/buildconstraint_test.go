package buildconstraint_test

import (
	"go/build/constraint"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/buildconstraint"
)

// TestNeverBuilt tests which //go:build lines exclude a file from every build.
// No build sets "ignore", and any other tag may be set or not.
func TestNeverBuilt(t *testing.T) {
	for line, want := range map[string]bool{
		"//go:build ignore":          true,
		"//go:build ignore && linux": true,
		"//go:build ignore || linux": false,
		"//go:build !ignore":         false,
		"//go:build linux":           false,
		"//go:build !linux":          false,
		"//go:build linux && !cgo":   false,
		"//go:build linux && !linux": true,
	} {
		t.Run(line, func(t *testing.T) {
			x, err := constraint.Parse(line)
			require.NoError(t, err)
			require.Equal(t, want, buildconstraint.NeverBuilt(x))
		})
	}
}
