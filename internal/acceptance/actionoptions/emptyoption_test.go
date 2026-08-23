// Asserts that an action expression is valid JavaScript whatever the options
// it is given evaluate to.

package acceptance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/actionoptions/app/datapagesgen/action"
)

// TestEmptyOptionIsSkipped covers an option helper given nothing to say,
// such as WithHeaders of an empty map: the expression carries no options object.
func TestEmptyOptionIsSkipped(t *testing.T) {
	tests := map[string]string{
		"a nil header map": action.POSTPageIndexSave(action.WithHeaders(nil)),
		"an empty header map": action.POSTPageIndexSave(
			action.WithHeaders(map[string]string{}),
		),
	}

	const plain = "@post('/save/')"
	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, plain, expr)
		})
	}
}

// TestOptionsAreWritten covers an empty option next to real ones:
// only the empty one is dropped.
func TestOptionsAreWritten(t *testing.T) {
	expr := action.POSTPageIndexSave(
		action.WithHeaders(nil),
		action.WithSelector("#out"),
		action.WithHeaders(map[string]string{"X-Trace": "abc"}),
	)

	require.Equal(t,
		`@post('/save/', {selector: '#out', headers: {'X-Trace': 'abc'}})`, expr)
}
