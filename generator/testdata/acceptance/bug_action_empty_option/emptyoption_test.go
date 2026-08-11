// Asserts that an action expression is valid JavaScript whatever the options
// it is given evaluate to.

package acceptance_test

import (
	"strings"
	"testing"

	"dpacceptance/datapagesgen/action"
)

// TestEmptyOptionIsSkipped is the recorded failure.
func TestEmptyOptionIsSkipped(t *testing.T) {
	tests := map[string]string{
		"a nil header map": action.POSTPageIndexSave(action.WithHeaders(nil)),
		"an empty header map": action.POSTPageIndexSave(
			action.WithHeaders(map[string]string{})),
	}

	const plain = "@post('/save/')"
	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(expr, "{: }") || strings.Contains(expr, ": ,") {
				t.Errorf("the expression carries an option with no name: %s", expr)
			}
			if expr != plain {
				t.Errorf(" got: %s\nwant: %s", expr, plain)
			}
		})
	}
}
