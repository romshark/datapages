package skeleton_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/generator/skeleton"
)

// TestCIWorkflowInstallsPinnedTools covers what the scaffolded workflow installs.
// An unpinned CLI regenerates with whatever released last,
// and the workflow fails the build when that differs from the committed code.
func TestCIWorkflowInstallsPinnedTools(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		version string
		want    string
	}{
		"release":           {version: "1.2.3", want: "datapages@v1.2.3"},
		"built from source": {version: "", want: "datapages@latest"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := skeleton.CIWorkflow(tc.version)
			require.NoError(t, err)
			require.Contains(t, got, "go install "+
				"github.com/romshark/datapages/cmd/"+tc.want)
			require.Contains(t, got, "go install "+skeleton.TemplCmd)
			require.False(t, strings.Contains(got, "templ@latest"),
				"templ must be pinned")
		})
	}
}
