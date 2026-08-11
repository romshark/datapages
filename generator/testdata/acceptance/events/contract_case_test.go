// Wires the events case into the shared contract suite.

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
	links: []string{
		href.PageIndex(),
		href.PageLog(),
		href.PageOther(),
		href.PageRoom(),
	},
	actions: []string{
		action.POSTPageIndexTick(),
		action.POSTPageRoomSay(),
		action.POSTPageRoomBroadcast(),
	},
	signalActions: []string{
		action.POSTPageIndexTick(),
		action.POSTPageRoomSay(),
		action.POSTPageRoomBroadcast(),
	},
	// optionedAction carries every option at once. The keys and their order
	// are asserted by the contract suite.
	optionedAction: action.POSTPageIndexTick(
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
	streamPath:     "/_$/",
	dispatchAction: action.POSTPageIndexTick(),
	dispatchBody:   `{"n":1}`,
}
