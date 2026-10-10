package offline_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestServiceWorkerCache tests how sw.js and sw.min.js, the copy the module serves,
// store the responses of requests that are not navigations, such as an img element
// loading a GET action: a response marked no-store or no-cache,
// and a refusal from the server. testdata/sw_test.mjs runs each script in Node.
func TestServiceWorkerCache(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	for _, script := range []string{"sw.js", "sw.min.js"} {
		t.Run(script, func(t *testing.T) {
			t.Parallel()
			out, err := exec.CommandContext(t.Context(),
				node, "testdata/sw_test.mjs", script).CombinedOutput()
			require.NoError(t, err, "%s", out)
		})
	}
}
