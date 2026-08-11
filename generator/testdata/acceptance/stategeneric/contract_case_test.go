// Wires the generic per-page state case into the shared contract suite.

package acceptance_test

import (
	"crypto/sha256"
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/action"
	"dpacceptance/datapagesgen/href"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

var contractCase = contract{
	newServer: func(
		t *testing.T, opts ...datapagesgen.ServerOption,
	) *datapagesgen.Server {
		t.Helper()
		key := sha256.Sum256([]byte("acceptance"))
		opts = append(opts, datapagesgen.WithStateConfig(
			datapagesgen.StateConfig{
				HMACKey: key[:],
				// Short enough for the suite to watch a tab's state expire.
				GracePeriod: contractStateGrace,
			},
		))
		return datapagesgen.NewServer(&app.App{}, inmem.New(8), opts...)
	},
	links: []string{
		href.PageIndex(),
		href.PageCount(),
		href.PageLabel(),
		href.PageEmbedOnly(),
	},
	actions: []string{
		action.POSTPageCountBump(),
		action.POSTPageLabelSet(),
	},
	signalActions: []string{action.POSTPageLabelSet()},
	// optionedAction carries every option at once.
	// The keys and their order are asserted by the contract suite.
	optionedAction: action.POSTPageCountBump(
		action.WithBefore("$busy = true"),
		action.WithContentType(action.ContentTypeForm),
		action.WithSelector("#it's"),
		action.WithHeaders(map[string]string{
			"X-Trace": "abc",
			// A value with a quote in it must not close the string it sits in,
			// and a second header must be separated from the first.
			"X-Note": "it's here",
		}),
		action.WithFilterSignals("name", "secret"),
		action.WithOpenWhenHidden(true),
		action.WithPayload("{id: $id}"),
		action.WithRetry(action.RetryAlways),
		action.WithRetryInterval(500),
		action.WithRetryScaler(1.5),
		action.WithRetryMaxWaitMs(30000),
		action.WithRetryMaxCount(3),
		action.WithRequestCancellation(action.RequestCancellationDisabled),
		action.WithAfter("$busy = false"),
	),
	// The instance id a page load hands out belongs to that page.
	// The contract therefore loads the page whose stream it opens.
	index:          "/count/",
	streamPath:     "/count/_$/",
	dispatchAction: action.POSTPageCountBump(),
	stateAction:    action.POSTPageCountBump(),
	stateGrace:     contractStateGrace,
}
