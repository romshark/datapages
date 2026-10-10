package delta_test

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/docs/delta"
)

// TestQuillDeltaVectors tests that the port matches quill-delta on the cases
// web/vectors.mjs wrote. Any difference makes documents diverge.
func TestQuillDeltaVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/quill-delta.json")
	require.NoError(t, err)
	var cases []struct {
		Doc                        delta.Delta `json:"doc"`
		A                          delta.Delta `json:"a"`
		B                          delta.Delta `json:"b"`
		Applied                    delta.Delta `json:"applied"`
		Transformed                delta.Delta `json:"transformed"`
		TransformedWithoutPriority delta.Delta `json:"transformedWithoutPriority"`
		Composed                   delta.Delta `json:"composed"`
		Positions                  [][2]int    `json:"positions"`
	}
	require.NoError(t, json.Unmarshal(b, &cases))
	require.NotEmpty(t, cases)

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			applied, err := c.Doc.Compose(c.A)
			require.NoError(t, err)
			require.Equal(t, c.Applied, applied, "doc.compose(a)")

			require.Equal(t, c.Transformed, c.A.Transform(c.B, true),
				"a.transform(b, true)")
			require.Equal(t, c.TransformedWithoutPriority, c.A.Transform(c.B, false),
				"a.transform(b, false)")

			composed, err := c.A.Compose(c.Transformed)
			require.NoError(t, err)
			require.Equal(t, c.Composed, composed, "a.compose(a.transform(b, true))")

			for p, want := range c.Positions {
				require.Equal(t, want[0], c.A.TransformPosition(p, true),
					"a.transformPosition(%d, true)", p)
				require.Equal(t, want[1], c.A.TransformPosition(p, false),
					"a.transformPosition(%d, false)", p)
			}
		})
	}
}
