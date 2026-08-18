package datapages_test

import (
	"context"
	"testing"

	"github.com/romshark/datapages"
)

// TestWithDispatchContext covers the only option a dispatch call takes.
// The generated closure builds the config with the handler's context and then
// applies the options, which is what the two cases below stand in for.
//
// The assertions are plain on purpose. A testify import here would be a test
// dependency of the root package, which every fixture module that replaces
// datapages would then have to carry in its go.sum.
func TestWithDispatchContext(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	handlerCtx := context.Background()
	ownCtx := context.WithValue(handlerCtx, ctxKey{}, "own")

	// The no-op case is about a nil context. It goes through a variable
	// because the literal is what staticcheck's SA1012 warns about.
	var nilCtx context.Context

	for name, tc := range map[string]struct {
		option datapages.DispatchOption
		expect context.Context
	}{
		"override": {
			option: datapages.WithDispatchContext(ownCtx),
			expect: ownCtx,
		},
		"nil keeps the default": {
			option: datapages.WithDispatchContext(nilCtx),
			expect: handlerCtx,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			conf := datapages.DispatchConfig{Context: handlerCtx}
			tc.option(&conf)
			if conf.Context != tc.expect {
				t.Errorf("got %v, want %v", conf.Context, tc.expect)
			}
		})
	}
}
