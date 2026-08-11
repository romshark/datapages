// Wires the smallest possible application into the shared contract suite.
//
// The case has no tests of its own. Its purpose is to run the contract
// against a model with nothing in it but one page and one event, which is
// where a generator that assumes a feature is present gives itself away.

package acceptance_test

import (
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
		return datapagesgen.NewServer(&app.App{}, inmem.New(8), opts...)
	},
	links:          []string{href.PageIndex()},
	streamPath:     "/_$/",
	dispatchAction: action.POSTPageIndexPing(),
	dispatchBody:   `{"n":1}`,
	actions:        []string{action.POSTPageIndexPing()},
	signalActions:  []string{action.POSTPageIndexPing()},
	optionedAction: action.POSTPageIndexPing(
		action.WithBefore("$busy = true"),
		action.WithContentType(action.ContentTypeForm),
		action.WithSelector("#it's"),
		action.WithHeaders(map[string]string{
			"X-Trace": "abc",
			// A value with a quote in it must not close the string it
			// sits in, and a second header must be separated from the
			// first.
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
}
